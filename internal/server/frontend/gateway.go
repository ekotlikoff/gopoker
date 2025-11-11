package gateway

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
	tableserver "github.com/ekotlikoff/gopoker/internal/server/backend"
	"github.com/gofrs/uuid"
	"github.com/gorilla/websocket"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	acceptableRequestPeriodMS   = 100
	maxBurstOfRequests          = 10
	maxTimeToWaitForRateLimiter = 2 * time.Second
	// Time allowed to read the next pong message from the peer
	pongWait = 5 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 7) / 10
)

// Types of updates that can be sent from a client
const (
	// RoundActionT is the PlayerRequestType corresponding to a RoundAction
	RoundActionT = PlayerRequestType(iota)
	// TableActionT is the PlayerRequestType corresponding to a TableAction
	TableActionT
)

// Types of updates that can be sent to a client
const (
	// RoundActionResponseT is the ServerToPlayerType corresponding to a RoundAction
	RoundActionResponseT = ServerToPlayerType(iota)
	// TableActionResponseT is the ServerToPlayerType corresponding to a TableActionResponse
	TableActionResponseT
	// PlayerUpdateT is the ServerToPlayerType corresponding to a PlayerUpdate
	PlayerUpdateT
)

var upgrader = websocket.Upgrader{}

var (
	sessionCache *TTLMap

	rateLimiter = make(chan time.Time, maxBurstOfRequests)

	//go:embed static
	webStaticFS embed.FS

	gatewayRateLimiterMetric = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "gopoker",
			Subsystem: "gateway",
			Name:      "rate_limiter_length",
			Help:      "Length of the rateLimiter channel.",
		},
		func() float64 {
			return float64(len(rateLimiter))
		},
	)

	gatewaySessionMetric = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "gopoker",
			Subsystem: "gateway",
			Name:      "session_count",
			Help:      "Total number of sessions in the cache.",
		},
		func() float64 {
			if sessionCache == nil {
				return 0
			}
			return float64(sessionCache.Len())
		},
	)

	gatewayResponseMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gopoker",
			Subsystem: "gateway",
			Name:      "request_total",
			Help:      "Total number of requests serviced.",
		},
		[]string{"uri", "method", "status"},
	)

	gatewayResponseDurationMetric = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gopoker",
			Subsystem: "gateway",
			Name:      "request_duration",
			Help:      "Duration of requests serviced.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
				2.5, 5, 10},
		},
		[]string{"uri", "method", "status"},
	)
)

func init() {
	sessionCache = NewTTLMap(50, 1800, 10)
	prometheus.MustRegister(gatewayResponseMetric)
	prometheus.MustRegister(gatewayResponseDurationMetric)
	prometheus.MustRegister(gatewaySessionMetric)
	prometheus.MustRegister(gatewayRateLimiterMetric)
}

type (
	// Gateway is the server that serves static files and proxies to the different
	// backends
	Gateway struct {
		TableServer *tableserver.TableServer
		BasePath    string
		Port        int
	}

	// Credentials for authentication
	Credentials struct {
		Username string
	}
	// ServerToPlayerType is the type of server to client comm
	ServerToPlayerType int
	// ServerToPlayer wraps the various types of communications that can be sent to the client
	ServerToPlayer struct {
		Type                ServerToPlayerType
		RoundActionResponse tableserver.RoundActionResponse
		TableActionResponse tableserver.TableActionResponse
		PlayerUpdate        *tableserver.PlayerUpdate
	}
	// PlayerRequestType is the type of reqeuest sent from the client.
	PlayerRequestType int
	// PlayerRequest are the requests players send to the server.
	PlayerRequest struct {
		Type        PlayerRequestType
		RoundAction model.RoundAction
		TableAction tableserver.TableAction
	}
)

// Serve static files and proxy to the different backends
func (gw *Gateway) Serve() {
	cleanupChan := make(chan struct{})
	setupRateLimiter(cleanupChan)
	mux := http.NewServeMux()
	bp := gw.BasePath
	if len(bp) > 0 && (bp[len(bp)-1:] == "/" || bp[0:1] != "/") {
		panic("Invalid gateway base path")
	}
	middleware := func(handler http.Handler) http.HandlerFunc {
		return prometheusMiddleware(rateLimiterMiddleware(handler))
	}
	mux.Handle(bp+"/", middleware(http.HandlerFunc(gw.handleWebRoot)))
	mux.Handle(bp+"/gopokerclient.wasm", middleware(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, os.Getenv("HOME")+"/bin/gopokerclient.wasm")
		})))
	mux.Handle(bp+"/session", middleware(http.HandlerFunc(Session)))
	mux.Handle(bp+"/tables", middleware(http.HandlerFunc(gw.Tables)))
	mux.Handle(bp+"/ws", middleware(http.HandlerFunc(gw.Websocket)))
	mux.Handle(bp+"/login", middleware(http.HandlerFunc(gw.login)))
	// Prometheus metrics endpoint
	mux.Handle(bp+"/metrics", middleware(
		promhttp.Handler()))
	log.Println("Gateway server listening on port", gw.Port, "...")
	http.ListenAndServe(":"+strconv.Itoa(gw.Port), mux)
	close(cleanupChan)
}

func setupRateLimiter(cleanupChan chan struct{}) {
	for i := 0; i < maxBurstOfRequests; i++ {
		rateLimiter <- time.Now()
	}
	go func() {
		ticker := time.NewTicker(acceptableRequestPeriodMS * time.Millisecond)
		defer ticker.Stop()
		for t := range ticker.C {
			select {
			case rateLimiter <- t:
			case <-cleanupChan:
				return
			}
		}
	}()
}

func (gw *Gateway) handleWebRoot(w http.ResponseWriter, r *http.Request) {
	bp := gw.BasePath
	if len(bp) > 0 && len(r.URL.Path) > len(bp) && r.URL.Path[0:len(bp)] == bp {
		r.URL.Path = "/static" + r.URL.Path[len(bp):]
	} else {
		r.URL.Path = "/static" + r.URL.Path // This is a hack to get the embedded path
	}
	http.FileServer(http.FS(webStaticFS)).ServeHTTP(w, r)
}

// Tables handles the /tables endpoint
func (gw *Gateway) Tables(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gw.getTables(w, r)
	case http.MethodPost:
		gw.createTable(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (gw *Gateway) getTables(w http.ResponseWriter, r *http.Request) {
	ts := gw.TableServer.GetTables()
	tableSummaries := make([]model.TableSummary, 0, len(ts))
	for name, table := range ts {
		tableSummaries = append(tableSummaries, model.TableSummary{
			Name:         name,
			PlayerCount:  table.PlayerCount(),
			StanderCount: table.StanderCount(),
			IsPlaying:    table.IsPlaying(),
		})
	}
	if err := json.NewEncoder(w).Encode(tableSummaries); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (gw *Gateway) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	player := getPlayerFromSession(r)

	if player == nil {
		if req.Username == "" {
			http.Error(w, "Missing username", http.StatusBadRequest)
			return
		}
		sessionToken, err := uuid.NewV4()
		if err != nil {
			http.Error(w, "Failed to generate session token", http.StatusInternalServerError)
			return
		}
		sessionTokenStr := sessionToken.String()
		player = tableserver.NewPlayer(req.Username)
		err = sessionCache.Put(sessionTokenStr, player)
		if errors.Is(err, ErrUsernameTaken) {
			http.Error(w, "Username Taken", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "Failed to store session token in sessionCache", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Value:   sessionTokenStr,
			Expires: time.Now().Add(1800 * time.Second),
		})
	}
}

func (gw *Gateway) createTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	player := getPlayerFromSession(r)

	if player == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	action := tableserver.CreateTableAction(req.Name, player)
	gw.TableServer.SendTableAction(action)
	resp := player.GetTableResponse()
	if resp.Err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(resp.Err.Error()))
		return
	}
	gw.TableServer.SendTableAction(tableserver.JoinTableAction(req.Name, player))
	resp = player.GetTableResponse()
	if resp.Err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(resp.Err.Error()))
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Session credit to https://www.sohamkamani.com/blog/2018/03/25/golang-session-authentication/
func Session(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		getSession(w, r)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func getSession(w http.ResponseWriter, r *http.Request) {
	tracer := opentracing.GlobalTracer()
	SessionSpan := tracer.StartSpan("GETSession")
	defer SessionSpan.Finish()
	player := GetSession(w, r)
	var currentMatchResponse SessionResponse
	if player == nil {
		return
	} else if player.GetTable() == nil {
		log.Println("Found session,", player.GetName())
		currentMatchResponse = SessionResponse{
			Credentials: Credentials{Username: player.GetName()},
		}
	} else {
		table := player.GetTable()
		currentMatchResponse = SessionResponse{
			Credentials: Credentials{Username: player.GetName()},
			AtTable:     true,
			Table:       table.SerializableTable(player),
		}
	}
	if err := json.NewEncoder(w).Encode(currentMatchResponse); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func getPlayerFromSession(r *http.Request) *tableserver.Player {
	c, err := r.Cookie("session_token")
	if err != nil {
		return nil
	}
	sessionToken := c.Value
	player, err := sessionCache.Get(sessionToken)
	if err != nil {
		log.Println("ERROR token is invalid in cache:", err)
		return nil
	}
	return player
}

// GetSession credit to https://www.sohamkamani.com/blog/2018/03/25/golang-session-authentication/
func GetSession(w http.ResponseWriter, r *http.Request) *tableserver.Player {
	tracer := opentracing.GlobalTracer()
	getSessionSpan := tracer.StartSpan("GetSession")
	defer getSessionSpan.Finish()
	player := getPlayerFromSession(r)
	if player == nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Missing session_token"))
	}
	return player
}

// SessionResponse serializable struct to send client's session
type SessionResponse struct {
	Credentials Credentials
	AtTable     bool
	Table       tableserver.SerializableTable
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader and record the status for instrumentation
func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Hijack the connection
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

// Websocket handles the /ws endpoint
func (gw *Gateway) Websocket(w http.ResponseWriter, r *http.Request) {
	player := GetSession(w, r)
	if player == nil {
		log.Println("No player found for session")
		return
	}

	if player.GetTable() == nil {
		// If player hasn't already joined the table, do so now.
		table := r.URL.Query().Get("table")
		gw.TableServer.SendTableAction(tableserver.JoinTableAction(table, player))
		resp := player.GetTableResponse()
		if resp.Err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(resp.Err.Error()))
			return
		}
	}

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer c.Close()

	waitc := make(chan struct{})
	player.ClientConnectToPlayer()
	defer player.ClientDisconnectFromPlayer()

	go readLoop(c, player, gw.TableServer, waitc)
	writeLoop(c, player)
	<-waitc
	log.Println("Websocketserver disconnecting from client: " + player.GetName())
}

func writeLoop(c *websocket.Conn, player *tableserver.Player) {
	if err := c.WriteJSON(ServerToPlayer{Type: PlayerUpdateT, PlayerUpdate: player.NewFullUpdate()}); err != nil {
		log.Println("Write error:", err)
		return
	}
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		var update ServerToPlayer
		select {
		case u := <-player.TableUpdateChan:
			update.Type = PlayerUpdateT
			update.PlayerUpdate = u
		case u := <-player.RoundResponseChan():
			update.Type = RoundActionResponseT
			update.RoundActionResponse = u
		case u := <-player.TableResponseChan():
			update.Type = TableActionResponseT
			update.TableActionResponse = u
		case <-ticker.C:
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("FATAL Write PingMessage error:", err)
				return
			}
			continue
		}

		if err := c.WriteJSON(update); err != nil {
			log.Println("Write error:", err)
			return
		}
	}
}

func readLoop(c *websocket.Conn, player *tableserver.Player, ts *tableserver.TableServer, waitc chan struct{}) {
	defer c.Close()
	for {
		var req PlayerRequest
		if err := c.ReadJSON(&req); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Websocketserver read error: %v", err)
			}
			close(waitc)
			return
		}
		switch req.Type {
		case RoundActionT:
			player.SendRoundAction(req.RoundAction)
		case TableActionT:
			ts.SendTableAction(*req.TableAction.SetPlayer(player))
		}
	}
}

// rateLimiterMiddleware handles the request by first blocking until the rate
// limiter says it is acceptable to proceed.
func rateLimiterMiddleware(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-rateLimiter:
			handler.ServeHTTP(w, r)
		case <-time.After(maxTimeToWaitForRateLimiter):
			// TODO once we add per-session rate limiting we should consider a
			// different status code for this global rate limiting.
			w.WriteHeader(429)
		}
	}
}

// prometheusMiddleware handles the request by passing it to the real
// handler and creating time series with the request details
func prometheusMiddleware(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := statusWriter{ResponseWriter: w}
		handler.ServeHTTP(&sw, r)
		duration := time.Since(start)
		gatewayResponseMetric.WithLabelValues(
			r.URL.Path, r.Method, fmt.Sprintf("%d", sw.status)).Inc()
		gatewayResponseDurationMetric.WithLabelValues(r.URL.Path, r.Method,
			fmt.Sprintf("%d", sw.status)).Observe(duration.Seconds())
	}
}

// SetQuiet logging
func SetQuiet() {
	log.SetOutput(ioutil.Discard)
}
