package gateway

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
	tableserver "github.com/ekotlikoff/gopoker/internal/server/backend"
	"github.com/gofrs/uuid"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	acceptableRequestPeriodMS   = 100
	maxBurstOfRequests          = 10
	maxTimeToWaitForRateLimiter = 2 * time.Second
)

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
	// TODO mux.Handle(bp+"/ws", middleware(http.HandlerFunc(Websocket)))
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
	tables := gw.TableServer.GetTables()
	serializableTables := make([]model.SerializableTable, 0, len(tables))
	for name, table := range tables {
		serializableTables = append(serializableTables, model.SerializableTable{
			Name:         name,
			PlayerCount:  table.PlayerCount(),
			StanderCount: table.StanderCount(),
			IsPlaying:    table.IsPlaying(),
		})
	}
	if err := json.NewEncoder(w).Encode(serializableTables); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (gw *Gateway) createTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	player := getPlayerFromSession(r)

	if player == nil {
		if req.Username == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing username"))
			return
		}
		sessionToken, err := uuid.NewV4()
		if err != nil {
			log.Println("Failed to generate session token")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sessionTokenStr := sessionToken.String()
		player = tableserver.NewPlayer(req.Username)
		err = sessionCache.Put(sessionTokenStr, player)
		if err != nil {
			log.Println("Failed to store session token in sessionCache")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Value:   sessionTokenStr,
			Expires: time.Now().Add(1800 * time.Second),
		})
	}

	action := tableserver.CreateTableAction(req.Name, player)
	gw.TableServer.SendTableAction(action)
	resp := player.GetTableResponse()
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
			Table: CurrentTable{
				TableConfig: table.TableConfig(),
				Players:     table.Players(),
				DealerIndex: table.DealerIndex(),
				Standers:    table.Standers(),
				Hand:        table.Hand(),
			},
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

// CurrentMatch serializable struct to bring client up to speed
type CurrentTable struct {
	TableConfig tableserver.TableConfig
	Players     [model.MaxTableSize]*model.Player
	DealerIndex int
	Standers    map[string]*model.Player
	Hand        *model.Hand
}

// SessionResponse serializable struct to send client's session
type SessionResponse struct {
	Credentials Credentials
	AtTable     bool
	Table       CurrentTable
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
