package chessserver

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chehsunliu/poker"
	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

const (
	// Stand is a player's attempt to stand up from the table.
	Stand = TableActionType(iota)
	// Sit is a player's attempt to sit at the table.
	Sit
	// Leave is a player's attempt to leave a table.
	Leave
	// Join is a player's attempt to join a table.
	Join
	// Create is a player's attempt to create a new table.
	Create
	// Start is a player's attempt to start a table's play.
	Start
	// Pause is a player's attempt to pause a table's play.
	Pause
	// Unpause is a player's attempt to unpause a table's play.
	Unpause
	// Refresh is a request for full state, for example after a browser refresh.
	Refresh

	defaultTimeToBet        = time.Second * 30
	defaultTimeBetweenHands = time.Second * 5
)

// Various types of updates that can be sent to a client.
const (
	// NewHandUpdateT is the newest hand, and the player's hand.
	NewHandUpdateT = PlayerUpdateType(iota)
	// RoundUpdateT is the most recent RoundAction made by an opponent.
	RoundUpdateT = PlayerUpdateType(iota)
	// BetUpdateT is a notification to the current better that the table is awaiting their bet.
	BetUpdateT = PlayerUpdateType(iota)
	// DealUpdateT is the new set of community cards.
	DealUpdateT = PlayerUpdateType(iota)
	// HandOverUpdateT is the result of the latest hand.
	HandOverUpdateT = PlayerUpdateType(iota)
	// FullUpdateT is the full update including the entire table's state.
	FullUpdateT = PlayerUpdateType(iota)
	// TableUpdateT is a table action e.g. a pause, somewhat standing up, etc.
	TableUpdateT = PlayerUpdateType(iota)
	// StateUpdateT is an update to table state - table no longer playing, etc.
	StateUpdateT = PlayerUpdateType(iota)
)

type (
	Time interface {
		now() time.Time
		sleep(time.Duration)
		after(time.Duration) <-chan time.Time
	}
	// TableServerConfig defines the TableServer's behavior.
	TableServerConfig struct {
		maxConcurrentTables int
	}
	// TableConfig defines the TableServer's opinion of how a given Table should be run.
	TableConfig struct {
		timeToBet        time.Duration
		timeBetweenHands time.Duration
		modelConfig      model.TableConfig
	}
	// StateUpdate is an update to the table's state, for things that don't happen immediately or based
	// on a clear user action.
	// For example: folks looking to stand start standing after a hand, the table stops playing due to
	// too few players
	StateUpdate struct {
		PlayStopped bool
		NowStanding []string
	}
	// PlayerUpdateType is the type of table update sent to a client.
	PlayerUpdateType int
	// PlayerUpdate includes the various kinds of updates sent to a client.
	PlayerUpdate struct {
		// Type is the type of the update.
		Type PlayerUpdateType
		// Hole is the player's hole in the newest hand.
		Hole []poker.Card
		// BigBlind is the big blind for the current hand.
		BigBlind int
		// Dealer is the new hand's dealer.
		Dealer string
		// Board is the new set of community cards corresponding to a deal update.
		Board []poker.Card
		// Winners is the result of the latest hand.
		Winners []model.Winner
		// RoundAction is the round action for the current better.
		RoundAction model.RoundAction
		// CurrentBetter is the player that made the round action.
		CurrentBetter string
		// Table is the full model.Table.
		Table model.Table
		// TableAction is a table action that may be relevant to a client.
		TableAction TableAction
		// StateUpdate is an update to the table's state
		StateUpdate StateUpdate
	}
	// Player is a struct representing a client, containing channels for communications
	// between the client and the the TableServer.
	Player struct {
		playerModel *model.Player
		// Channel for requests from the client directed to the server.
		requestChan chan model.RoundAction
		// Channel for responses to requests from the client.
		responseChan chan RoundActionResponse
		// Channel for responses to requests from the client.
		tableResponseChan chan TableActionResponse
		// TableUpdateChan channel for updates to the Table's state.
		TableUpdateChan chan *PlayerUpdate
		// The player's current table if any
		table *Table
		// Only one client connected to the player at a time
		mutex sync.Mutex
	}
	// Table is an instance of an ongoing game the TableServer is hosting.
	Table struct {
		name        string
		tableConfig TableConfig
		adminName   string
		mutex       sync.Mutex
		table       *model.Table
		time        Time
		playing     bool
		pauseChan   chan struct{}
		unpauseChan chan struct{}
		players     map[string]*Player
	}
	// TableServer is the server that orchestrates one or more ongoing Tables.
	TableServer struct {
		tableServerConfig TableServerConfig
		tables            map[string]*Table
		tableActions      chan TableAction
		time              Time
		mutex             sync.Mutex
	}

	// TableActionType is the type of action a player can take on a table outside of an ongoing game.
	TableActionType int

	// TableAction is a player's request to the table outside the scope of a given round.
	TableAction struct {
		tableActionType TableActionType
		tableName       string
		seat            int
		tableConfig     TableConfig
		player          *Player
	}

	// TableActionResponse is a resposne to a client's TableAction.
	TableActionResponse struct {
		Err error
	}

	// RoundActionResponse is a resposne to a client's RoundAction.
	RoundActionResponse struct {
		Err error
	}
)

func defaultTableConfig() TableConfig {
	return TableConfig{
		timeToBet:        defaultTimeToBet,
		timeBetweenHands: defaultTimeBetweenHands,
		modelConfig:      model.DefaultConfig(),
	}
}

// NewPlayer creates a new Player.
func NewPlayer(name string) *Player {
	return &Player{
		playerModel:       model.NewPlayer(name),
		requestChan:       make(chan model.RoundAction),
		responseChan:      make(chan RoundActionResponse),
		tableResponseChan: make(chan TableActionResponse),
		TableUpdateChan:   make(chan *PlayerUpdate, 10),
	}
}

type realTime struct{}

func (realTime) now() time.Time                         { return time.Now() }
func (realTime) sleep(d time.Duration)                  { time.Sleep(d) }
func (realTime) after(d time.Duration) <-chan time.Time { return time.After(d) }

// NewTableServer creates a new TableServer.
func NewTableServer() *TableServer {
	return NewTableServerWithTime(realTime{})
}

// NewTableServerWithTime creates a new TableServer with specified time implementations.
func NewTableServerWithTime(t Time) *TableServer {
	return &TableServer{
		tableServerConfig: TableServerConfig{
			maxConcurrentTables: 5,
		},
		tables:       make(map[string]*Table),
		tableActions: make(chan TableAction),
		time:         t,
	}
}

// SendTableAction sends the TableServer an action.
func (ts *TableServer) SendTableAction(a TableAction) {
	ts.tableActions <- a
}

// Stop stops the table server
func (ts *TableServer) Stop() {
	close(ts.tableActions)
}

func (ts *TableServer) newTable(name string, config TableConfig, creator string) error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	if len(ts.tables) >= ts.tableServerConfig.maxConcurrentTables {
		return errors.New("too many tables")
	} else if _, ok := ts.tables[name]; ok {
		return errors.New("duplicate name")
	}
	ts.tables[name] = &Table{
		name: name, table: model.NewTableWithConfig(config.modelConfig),
		adminName: creator, tableConfig: config,
		pauseChan:   make(chan struct{}),
		unpauseChan: make(chan struct{}),
		players:     make(map[string]*Player),
		time:        ts.time,
	}
	return nil
}

// JoinTableAction makes a Join action.
func JoinTableAction(t string, p *Player) TableAction {
	return TableAction{
		tableActionType: Join,
		tableName:       t,
		player:          p,
	}
}

// SitTableAction makes a Sit action.
func SitTableAction(t string, p *Player, s int) TableAction {
	return TableAction{
		tableActionType: Sit,
		tableName:       t,
		player:          p,
		seat:            s,
	}
}

// StandTableAction makes a Stand action.
func StandTableAction(t string, p *Player) TableAction {
	return TableAction{
		tableActionType: Stand,
		tableName:       t,
		player:          p,
	}
}

// CreateTableAction makes a Create action.
func CreateTableAction(t string, p *Player) TableAction {
	return TableAction{
		tableActionType: Create,
		tableName:       t,
		tableConfig:     defaultTableConfig(),
		player:          p,
	}
}

// StartTableAction starts the table.
func StartTableAction(t string, p *Player) TableAction {
	return TableAction{
		tableActionType: Start,
		tableName:       t,
		player:          p,
	}
}

// PauseTableAction pauses the table.
func PauseTableAction(t string, p *Player) TableAction {
	return TableAction{
		tableActionType: Pause,
		tableName:       t,
		player:          p,
	}
}

// UnpauseTableAction unpauses the table.
func UnpauseTableAction(t string, p *Player) TableAction {
	return TableAction{
		tableActionType: Unpause,
		tableName:       t,
		player:          p,
	}
}

// Serve starts the table server
func (ts *TableServer) Serve() {
	for a := range ts.tableActions {
		var err error
		switch a.tableActionType {
		case Create:
			err = ts.newTable(
				a.tableName,
				a.tableConfig,
				a.player.playerModel.Name,
			)
		case Stand:
			ts.mutex.Lock()
			a.player.playerModel.StandUp()
			a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			ts.mutex.Unlock()
		case Sit:
			ts.mutex.Lock()
			p := a.player
			var table *Table
			if table = ts.tables[a.tableName]; table == nil {
				p.tableResponseChan <- TableActionResponse{
					fmt.Errorf("no table %q", a.tableName),
				}
				continue
			}
			err = table.table.SitDown(
				a.player.playerModel, a.seat,
			)
			if err == nil {
				p.table = table
				table.players[p.playerModel.Name] = p
				a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			}
			ts.mutex.Unlock()
		case Leave:
			ts.mutex.Lock()
			err = a.player.playerModel.Leave()
			if err == nil {
				a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			}
			ts.mutex.Unlock()
		case Join:
			ts.mutex.Lock()
			err = fmt.Errorf("table %q does not exist", a.tableName)
			for _, t := range ts.tables {
				if t.name == a.tableName {
					err = a.player.join(t)
					break
				}
			}
			if err == nil {
				a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			}
			ts.mutex.Unlock()
		case Start:
			err = ts.start(a)
		case Pause:
			// TODO who should be allowed to pause?
			a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			ts.tables[a.tableName].pauseChan <- struct{}{}
		case Unpause:
			a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			ts.tables[a.tableName].unpauseChan <- struct{}{}
		case Refresh:
			// TODO
			log.Fatalf("not implemented")
			a.player.TableUpdateChan <- &PlayerUpdate{
				Type: FullUpdateT,
			}
		}
		a.player.tableResponseChan <- TableActionResponse{err}
	}
}

// GetTable get's the player's current table.
func (p *Player) GetTable() *Table {
	p.mutex.Lock()
	p.mutex.Unlock()
	return p.table
}

// GetTableResponse gets a response from the table.
func (p *Player) GetTableResponse() TableActionResponse {
	return <-p.tableResponseChan
}

// SendRoundAction sends a round action to the server.
func (p *Player) SendRoundAction(a model.RoundAction) {
	p.requestChan <- a
}

// GetRoundResponse gets a response from the table.
func (p *Player) GetRoundResponse() RoundActionResponse {
	return <-p.responseChan
}

func (p *Player) join(t *Table) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	t.mutex.Lock()
	defer t.mutex.Unlock()
	err := t.table.Join(p.playerModel)
	if err == nil {
		t.players[p.playerModel.Name] = p
		p.table = t
	}
	return err
}

func (t *Table) isPlaying() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.playing
}

func (t *Table) setPlaying(p bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.playing = p
}

func (t *Table) getBoard() []poker.Card {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Board()
}

func (t *Table) getTimeToBet() time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.tableConfig.timeToBet
}

func (t *Table) getTimeBetweenHands() time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.tableConfig.timeBetweenHands
}

func (ts *TableServer) start(a TableAction) error {
	ts.mutex.Lock()
	table, ok := ts.tables[a.tableName]
	ts.mutex.Unlock()
	if !ok {
		return fmt.Errorf("no such table %q", a.tableName)
	} else if a.player.playerModel.Name != table.adminName {
		return fmt.Errorf("%s is not the admin, %s is", a.player.playerModel.Name, table.adminName)
	}
	table.sendPlayerUpdates(newTableUpdate(a))
	go ts.serveTable(table)
	return nil
}

func (t *Table) dealer() string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Dealer().Name
}

func (t *Table) bigBlindAmount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.BigBlindAmount()
}

func (ts *TableServer) serveTable(t *Table) error {
	if t.isPlaying() {
		return errors.New("play: table already playing")
	}
	t.setPlaying(true)
	for {
		t.handlePause()
		t.table.NewHand()
		if err := t.table.StartHand(); err != nil {
			t.sendPlayerUpdates(newStateUpdate(true, nil))
			t.setPlaying(false)
			log.Print(err)
			return err
		}
		log.Println("new hand started")
		t.sendNewHandUpdates(t.bigBlindAmount(), t.dealer())
		t.handlePause()
		t.listenForPlayerActions()
		for !t.table.HandDone() {
			t.table.Deal()
			t.sendPlayerUpdates(newDealUpdate(t.getBoard()))
			t.listenForPlayerActions()
			if len(t.table.Board()) == 5 {
				t.table.SetHandDone(true)
			}
		}
		err, winners := t.table.FinishHand()
		if err != nil {
			t.setPlaying(false)
			log.Println(err)
			return err
		}
		log.Println("hand over")
		t.sendPlayerUpdates(newHandOverUpdate(winners))
		standers := t.table.HandleStanders()
		if standers != nil {
			t.sendPlayerUpdates(newStateUpdate(false, standers))
		}
		t.time.sleep(t.getTimeBetweenHands())

	}
}

func (t *Table) handlePause() {
	select {
	case <-t.pauseChan:
		<-t.unpauseChan
	default:
	}

}

func (t *Table) sendNewHandUpdates(bb int, d string) {
	var wg sync.WaitGroup
	for _, p := range t.players {
		p.sendPlayerUpdate(
			&PlayerUpdate{
				Type:     NewHandUpdateT,
				Hole:     p.playerModel.Hole,
				BigBlind: bb,
				Dealer:   d,
			}, &wg)
	}
	wg.Wait()
}

func newDealUpdate(board []poker.Card) *PlayerUpdate {
	return &PlayerUpdate{
		Type:  DealUpdateT,
		Board: board,
	}
}

func newHandOverUpdate(winners []model.Winner) *PlayerUpdate {
	return &PlayerUpdate{
		Type:    HandOverUpdateT,
		Winners: winners,
	}
}

func newStateUpdate(playStopped bool, nowStanding []string) *PlayerUpdate {
	return &PlayerUpdate{
		Type: StateUpdateT,
		StateUpdate: StateUpdate{
			PlayStopped: playStopped,
			NowStanding: nowStanding,
		},
	}
}

func newTableUpdate(a TableAction) *PlayerUpdate {
	return &PlayerUpdate{
		Type:        TableUpdateT,
		TableAction: a,
	}
}

func newBetUpdate() *PlayerUpdate {
	return &PlayerUpdate{
		Type: BetUpdateT,
	}
}

func newRoundUpdate(a model.RoundAction, p *model.Player) *PlayerUpdate {
	return &PlayerUpdate{
		Type:          RoundUpdateT,
		RoundAction:   a,
		CurrentBetter: p.Name,
	}
}

func (t *Table) listenForPlayerActions() {
	for !t.table.RoundDone() && !t.table.BettingDone() && !t.table.HandDone() {
		success := false
		player := t.table.CurrentBetter()
		timeRemaining := t.getTimeToBet()
		for !success {
			client := t.players[player.Name]
			a, elapsedTime := getPlayerAction(timeRemaining, client, t)
			err := t.table.HandlePlayerAction(player, a)
			timeRemaining -= elapsedTime
			if err == nil {
				t.sendPlayerUpdates(newRoundUpdate(a, player))
				success = true
			} else {
				log.Println(err)
			}
			client.responseChan <- RoundActionResponse{err}
		}
		log.Println(player.Name, "made their bet")
	}
	log.Println("Round of betting is done")
	t.table.SetRoundDone(true)
}

func (t *Table) sendPlayerUpdates(u *PlayerUpdate) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	var wg sync.WaitGroup
	for _, p := range t.players {
		p.sendPlayerUpdate(u, &wg)
	}
	wg.Wait()
}

func (p *Player) sendPlayerUpdate(u *PlayerUpdate, wg *sync.WaitGroup) {
	wg.Add(1)
	go func(p *Player) {
		select {
		case p.TableUpdateChan <- u:
		case <-p.table.time.after(500 * time.Millisecond):
			log.Printf("time out sending to %s's tableUpdateChan", p.playerModel.Name)
		}
		wg.Done()
	}(p)
}

// Returns the selected player action and the elapsed time (not counting any pause)
func getPlayerAction(timeRemaining time.Duration, player *Player, t *Table) (model.RoundAction, time.Duration) {
	log.Println("Waiting for action from", player.playerModel.Name)
	action := model.RoundAction{ActionType: model.Fold}
	n := t.time.now()
	var elapsedTime time.Duration
	for {
		var wg sync.WaitGroup
		afterChan := t.time.after(timeRemaining - elapsedTime)
		player.sendPlayerUpdate(newBetUpdate(), &wg)
		wg.Wait()
		select {
		case <-t.pauseChan:
			elapsedTime += t.time.now().Sub(n)
			<-t.unpauseChan
		case action = <-player.requestChan:
			return action, elapsedTime + t.time.now().Sub(n)
		case <-afterChan:
			log.Println(player.playerModel.Name, "timed out, folding")
			return action, timeRemaining
		}
	}
}

func (u *PlayerUpdate) String() string {
	var out string
	out += fmt.Sprintf("%s: ", u.Type)
	switch u.Type {
	case DealUpdateT:
		out += fmt.Sprint(u.Board)
	case FullUpdateT:
	case RoundUpdateT:
		out += fmt.Sprintf("%s did %v\n", u.CurrentBetter, u.RoundAction)
	case StateUpdateT:
		out += fmt.Sprintf("%+v\n", u.StateUpdate)
	case TableUpdateT:
		out += fmt.Sprintf("%+v\n", u.TableAction)
	case NewHandUpdateT:
		out += fmt.Sprintf("hole: %v, dealer: %s, big blind: %d\n", u.Hole, u.Dealer, u.BigBlind)
	case HandOverUpdateT:
		out += fmt.Sprintf("%+v\n", u.Winners)
	case BetUpdateT:
		out += fmt.Sprintf("%v\n", u.Type)
	default:
		return "Unknown"
	}
	return out
}

func (u PlayerUpdateType) String() string {
	switch u {
	case DealUpdateT:
		return "DealUpdateT"
	case FullUpdateT:
		return "FullUpdateT"
	case RoundUpdateT:
		return "RoundUpdateT"
	case StateUpdateT:
		return "StateUpdateT"
	case TableUpdateT:
		return "TableUpdateT"
	case NewHandUpdateT:
		return "NewHandUpdateT"
	case BetUpdateT:
		return "BetUpdateT"
	case HandOverUpdateT:
		return "HandOverUpdateT"
	default:
		return "Unknown"
	}
}

func (t TableActionType) String() string {
	switch t {
	case Stand:
		return "Stand"
	case Sit:
		return "Sit"
	case Leave:
		return "Leave"
	case Join:
		return "Join"
	case Create:
		return "Create"
	case Start:
		return "Start"
	case Pause:
		return "Pause"
	case Unpause:
		return "Unpause"
	case Refresh:
		return "Refresh"
	default:
		return "Unknown"
	}
}
