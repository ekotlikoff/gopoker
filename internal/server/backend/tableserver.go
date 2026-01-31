package tableserver

import (
	"errors"
	"fmt"
	"log"
	"math"
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
	maxTimeBetweenHands     = time.Second * 15
	defaultTimeBetweenDeals = time.Second * 2
	maxConcurrentTables     = 10
)

// Various types of updates that can be sent to a client.
const (
	// NewHandUpdateT is the newest hand, and the player's hand.
	NewHandUpdateT = PlayerUpdateType(iota)
	// RoundUpdateT is the most recent RoundAction made by an opponent.
	RoundUpdateT
	// BetUpdateT is a notification to the current better that the table is awaiting their bet.
	BetUpdateT
	// DealUpdateT is the new set of community cards.
	DealUpdateT
	// PotUpdateT is the latest pot, divied up between potential side pots.
	PotUpdateT
	// HandOverUpdateT is the result of the latest hand.
	HandOverUpdateT
	// FullUpdateT is the full update including the entire table's state.
	FullUpdateT
	// TableUpdateT is a table action e.g. a pause, someone standing up, etc.
	TableUpdateT
	// StateUpdateT is an update to table state - table no longer playing, etc.
	StateUpdateT
)

type (
	clock interface {
		now() time.Time
		sleep(time.Duration)
		after(time.Duration) <-chan time.Time
	}
	// TableConfig defines the TableServer's opinion of how a given Table should be run.
	TableConfig struct {
		TimeToBet        time.Duration
		TimeBetweenHands time.Duration
		TimeBetweenDeals time.Duration
		ModelConfig      model.TableConfig
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
		// TimeBetweenHands is the amount of time the server will take before starting the next hand.
		TimeBetweenHands time.Duration
		// RoundAction is the round action for the current better.
		RoundAction model.RoundAction
		// CurrentBetter is the player that made the round action.
		CurrentBetter string
		// CurrentSeat is the relevant seat.
		CurrentSeat int
		// TimeRemaining to bet
		TimeRemaining time.Duration
		// TimeElapsed in bet
		TimeElapsed time.Duration
		// CurrentBets is the current bets from each player.
		CurrentBets []int
		// CurrentFunds is the current funds for each player.
		CurrentFunds []int
		// Pot is the pot(s) in the hand.
		Pot model.Pot
		// RoundDone marks whether the round of betting is done or not.
		RoundDone bool
		// RoundEndedWithFold marks whether the round of betting ended with a fold or not.
		RoundEndedWithFold bool
		// HandDone marks whether the hand is done.
		HandDone bool
		// Table contains the full table state, sent upon first connection.
		Table SerializableTable
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
		// True if there is a client connected to the player.
		connected bool
	}
	// SerializableTable is a serializable version of the table that contains information clients need.
	SerializableTable struct {
		Name        string
		TableConfig TableConfig
		AdminName   string
		Table       model.SerializableTable
		Playing     bool
		Paused      bool
	}
	// Table is an instance of an ongoing game the TableServer is hosting.
	Table struct {
		name        string
		tableConfig TableConfig
		adminName   string
		mutex       sync.Mutex
		table       *model.Table
		clock       clock
		playing     bool
		pauseChan   chan struct{}
		unpauseChan chan struct{}
		paused      bool
		players     map[string]*Player
	}
	// TableServer is the server that orchestrates one or more ongoing Tables.
	TableServer struct {
		tables       map[string]*Table
		tableActions chan TableAction
		clock        clock
		mutex        sync.Mutex
	}

	// TableActionType is the type of action a player can take on a table outside of an ongoing game.
	TableActionType int

	// TableAction is a player's request to the table outside the scope of a given round.
	TableAction struct {
		TableActionType TableActionType
		TableName       string
		Seat            int
		TableConfig     TableConfig
		PlayerName      string
		player          *Player
	}

	// TableActionResponse is a resposne to a client's TableAction.
	TableActionResponse struct {
		Err         string
		TableAction TableAction
	}

	// RoundActionResponse is a resposne to a client's RoundAction.
	RoundActionResponse struct {
		Err string
	}
)

func defaultTableConfig() TableConfig {
	return TableConfig{
		TimeToBet:        defaultTimeToBet,
		TimeBetweenHands: defaultTimeBetweenHands,
		TimeBetweenDeals: defaultTimeBetweenDeals,
		ModelConfig:      model.DefaultConfig(),
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
func NewTableServerWithTime(c clock) *TableServer {
	return &TableServer{
		tables:       make(map[string]*Table),
		tableActions: make(chan TableAction),
		clock:        c,
	}
}

// GetTables gets all the current tables
func (ts *TableServer) GetTables() map[string]*Table {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	return ts.tables
}

// SetPlayer sets the player on the TableAction
func (a *TableAction) SetPlayer(p *Player) *TableAction {
	a.player = p
	return a
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
	if len(ts.tables) >= maxConcurrentTables {
		return errors.New("too many tables")
	} else if _, ok := ts.tables[name]; ok {
		return errors.New("duplicate name")
	}
	ts.tables[name] = &Table{
		name: name, table: model.NewTableWithConfig(config.ModelConfig),
		adminName: creator, tableConfig: config,
		pauseChan:   make(chan struct{}),
		unpauseChan: make(chan struct{}),
		players:     make(map[string]*Player),
		clock:       ts.clock,
	}
	return nil
}

// JoinTableAction makes a Join action.
func JoinTableAction(t string, p *Player) TableAction {
	return TableAction{
		TableActionType: Join,
		TableName:       t,
		PlayerName:      p.GetName(),
		player:          p,
	}
}

// SitTableAction makes a Sit action.
func SitTableAction(t string, p *Player, s int) TableAction {
	return TableAction{
		TableActionType: Sit,
		TableName:       t,
		PlayerName:      p.GetName(),
		player:          p,
		Seat:            s,
	}
}

// StandTableAction makes a Stand action.
func StandTableAction(t string, p *Player) TableAction {
	return TableAction{
		TableActionType: Stand,
		TableName:       t,
		PlayerName:      p.GetName(),
		player:          p,
		Seat:            p.GetSeat(),
	}
}

// CreateTableAction makes a Create action.
func CreateTableAction(t string, p *Player) TableAction {
	return TableAction{
		TableActionType: Create,
		TableName:       t,
		TableConfig:     defaultTableConfig(),
		PlayerName:      p.GetName(),
		player:          p,
	}
}

// StartTableAction starts the table.
func StartTableAction(t string, p *Player) TableAction {
	return TableAction{
		TableActionType: Start,
		TableName:       t,
		PlayerName:      p.GetName(),
		player:          p,
	}
}

// PauseTableAction pauses the table.
func PauseTableAction(t string, p *Player) TableAction {
	return TableAction{
		TableActionType: Pause,
		TableName:       t,
		PlayerName:      p.GetName(),
		player:          p,
	}
}

// UnpauseTableAction unpauses the table.
func UnpauseTableAction(t string, p *Player) TableAction {
	return TableAction{
		TableActionType: Unpause,
		TableName:       t,
		PlayerName:      p.GetName(),
		player:          p,
	}
}

// Serve starts the table server
func (ts *TableServer) Serve() {
	for a := range ts.tableActions {
		var err error
		switch a.TableActionType {
		case Create:
			err = ts.newTable(
				a.TableName,
				a.TableConfig,
				a.player.playerModel.Name,
			)
		case Stand:
			ts.mutex.Lock()
			if a.player.GetTable().IsPlaying() {
				a.player.StandUp()
			} else {
				err = a.player.StandNow()
				if err == nil {
					a.Seat = a.player.GetSeat()
					a.PlayerName = a.player.GetName()
					a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
				}
			}
			ts.mutex.Unlock()
		case Sit:
			ts.mutex.Lock()
			p := a.player
			var table *Table
			if table = ts.tables[a.TableName]; table == nil {
				p.tableResponseChan <- TableActionResponse{
					fmt.Errorf("no table %q", a.TableName).Error(),
					a,
				}
				continue
			}
			err = table.table.SitDown(
				a.player.playerModel, a.Seat,
			)
			if err == nil {
				p.mutex.Lock()
				p.table = table
				table.players[p.playerModel.Name] = p
				a.PlayerName = p.playerModel.Name
				table.sendPlayerUpdates(newTableUpdate(a))
				p.mutex.Unlock()
			}
			ts.mutex.Unlock()
		case Leave:
			ts.mutex.Lock()
			t := a.player.GetTable()
			err = a.player.Leave()
			if err == nil {
				t.sendPlayerUpdates(newTableUpdate(a))
				a.player.table = nil
			}
			ts.mutex.Unlock()
		case Join:
			ts.mutex.Lock()
			err = fmt.Errorf("table %q does not exist", a.TableName)
			for _, t := range ts.tables {
				if t.name == a.TableName {
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
			ts.tables[a.TableName].pauseChan <- struct{}{}
		case Unpause:
			a.player.GetTable().sendPlayerUpdates(newTableUpdate(a))
			ts.tables[a.TableName].unpauseChan <- struct{}{}
		case Refresh:
			// TODO
			log.Fatalf("not implemented")
			a.player.TableUpdateChan <- &PlayerUpdate{
				Type: FullUpdateT,
			}
		}
		a.player.tableResponseChan <- TableActionResponse{errorToString(err), a}
	}
}

// GetName get's the player's name.
func (p *Player) GetName() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.playerModel.Name
}

// GetSeat get's the player's seat.
func (p *Player) GetSeat() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.playerModel.SeatIndex
}

// GetTable get's the player's current table.
func (p *Player) GetTable() *Table {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.table
}

// StandUp from the table
func (p *Player) StandUp() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.playerModel.StandUp()
}

// StandNow from the table
func (p *Player) StandNow() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.playerModel.StandNow()
}

// Leave the table
func (p *Player) Leave() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.playerModel.Leave()
}

// TableResponseChan provides the player's TableResponseChan
func (p *Player) TableResponseChan() <-chan TableActionResponse {
	return p.tableResponseChan
}

// RoundResponseChan provides the player's RoundResponseChan
func (p *Player) RoundResponseChan() <-chan RoundActionResponse {
	return p.responseChan
}

// GetTableResponse gets a response from the table.
func (p *Player) GetTableResponse() TableActionResponse {
	return <-p.tableResponseChan
}

// SendRoundAction sends a round action to the server.
func (p *Player) SendRoundAction(a model.RoundAction) {
	if p.table.paused {
		return
	}
	p.requestChan <- a
}

// GetRoundResponse gets a response from the table.
func (p *Player) GetRoundResponse() RoundActionResponse {
	return <-p.responseChan
}

// ClientConnectToPlayer connects a client to the player.
func (p *Player) ClientConnectToPlayer() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.connected = true
}

// ClientDisconnectFromPlayer disconnects a client from the player.
func (p *Player) ClientDisconnectFromPlayer() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.connected = false
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

// IsPlaying returns whether the table is playing or not
func (t *Table) IsPlaying() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.playing
}

// Players returns the players at the table
func (t *Table) Players() [model.MaxTableSize]*model.Player {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Players
}

// Standers returns the standers at the table
func (t *Table) Standers() map[string]*model.Player {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Standers
}

// Hand returns the table's hand
func (t *Table) Hand() *model.Hand {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand
}

// TableConfig returns the table's config
func (t *Table) TableConfig() TableConfig {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.tableConfig
}

// SerializableTable creates a serializable version of the table state for a player, with only the information that player needs.
func (t *Table) SerializableTable(p *Player) SerializableTable {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// TODO remove details of other players hands that this player should not know.
	return SerializableTable{
		Name:        t.name,
		TableConfig: t.tableConfig,
		AdminName:   t.adminName,
		Table:       t.table.SerializableTable(p.playerModel.Name),
		Playing:     t.playing,
		Paused:      t.paused,
	}
}

func (t *Table) sanitizeAction(action model.RoundAction, player *Player) model.RoundAction {
	switch action.ActionType {
	case model.Raise:
		if action.Bet == 0 {
			action.ActionType = model.Check
		} else if action.Bet == t.Hand().Round.CurrentBet {
			action.ActionType = model.Call
		}
		return action
	case model.AllIn:
		action.Bet = player.playerModel.Funds + player.playerModel.BetAmount
	}
	return action
}

// DealerIndex returns the table's dealer index
func (t *Table) DealerIndex() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.DealerIndex
}

// PlayerCount returns the count of players sitting at the table
func (t *Table) PlayerCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	count := 0
	for _, p := range t.table.Players {
		if p != nil {
			count++
		}
	}
	return count
}

// StanderCount gets the count of standers at the table
func (t *Table) StanderCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return len(t.table.Standers)
}

// Name gets the name of the table
func (t *Table) Name() string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.name
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
	return t.tableConfig.TimeToBet
}

func (t *Table) getTimeBetweenHands(finalBetterCount int, winners []model.Winner) time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	var totalPot int
	var totalAllInners int
	for _, w := range winners {
		totalPot += w.Winnings
		if w.Player.Funds == 0 {
			totalAllInners++
		}
	}
	multiplier := 1.0
	// Increase wait time by 20% for every 2 final betters
	multiplier *= max(1.0, math.Pow(1.2, float64(finalBetterCount/2)))
	// Increase wait time by 20% for every DefaultFunds in the pot
	multiplier *= max(1.0, math.Pow(1.02, float64(totalPot/(t.tableConfig.ModelConfig.DefaultFunds/10.0))))
	// Increase wait time by 50% for every all in
	multiplier *= max(1.0, math.Pow(1.5, float64(totalAllInners)))
	return min(maxTimeBetweenHands, time.Duration(float64(t.tableConfig.TimeBetweenHands)*multiplier))
}

func (t *Table) getTimeBetweenDeals() time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.tableConfig.TimeBetweenDeals
}

func (ts *TableServer) start(a TableAction) error {
	ts.mutex.Lock()
	table, ok := ts.tables[a.TableName]
	ts.mutex.Unlock()
	if table.PlayerCount() < 2 {
		return fmt.Errorf("insufficient players")
	}
	if !ok {
		return fmt.Errorf("no such table %q", a.TableName)
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
	if t.IsPlaying() {
		return errors.New("play: table already playing")
	}
	t.setPlaying(true)
	for {
		t.handlePause()
		if err := t.table.NewHand(); err != nil {
			t.sendPlayerUpdates(newStateUpdate(true, nil))
			t.setPlaying(false)
			log.Print(err)
			return err
		}
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
			t.handlePause()
			t.clock.sleep(t.getTimeBetweenDeals())
			t.table.Deal()
			t.sendPlayerUpdates(newDealUpdate(t.getBoard()))
			t.listenForPlayerActions()
			if len(t.table.Board()) == 5 {
				t.table.SetHandDone(true)
			}
		}
		winners, finalBetterCount, err := t.table.FinishHand()
		if err != nil {
			t.setPlaying(false)
			log.Println(err)
			return err
		}
		log.Println("hand over")
		timeBetweenHands := t.getTimeBetweenHands(finalBetterCount, winners)
		t.sendPlayerUpdates(newHandOverUpdate(winners, timeBetweenHands, t))
		standers := t.table.HandleStanders()
		t.clock.sleep(timeBetweenHands)
		for _, p := range standers {
			a := StandTableAction(t.name, t.players[p])
			t.sendPlayerUpdates(newTableUpdate(a))
			t.sendPlayerUpdates(newStateUpdate(false, standers))
		}

	}
}

func (t *Table) handlePause() {
	select {
	case <-t.pauseChan:
		t.updatePaused(true)
		<-t.unpauseChan
		log.Println("unpaused")
		t.updatePaused(false)
	default:
	}

}

func (t *Table) updatePaused(p bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.paused = p
}

func (t *Table) currentBets() []int {
	players := t.table.GetPlayers()
	out := make([]int, len(players))
	for i, p := range players {
		if p != nil {
			out[i] = p.BetAmount
		}
	}
	return out
}

func (t *Table) currentFunds() []int {
	players := t.table.GetPlayers()
	out := make([]int, len(players))
	for i, p := range players {
		if p != nil {
			out[i] = p.Funds
		}
	}
	return out
}

func (t *Table) sendNewHandUpdates(bb int, d string) {
	var wg sync.WaitGroup
	for _, p := range t.players {
		p.sendPlayerUpdate(
			&PlayerUpdate{
				Type:         NewHandUpdateT,
				Hole:         p.playerModel.Hole,
				BigBlind:     bb,
				Dealer:       d,
				CurrentBets:  t.currentBets(),
				CurrentFunds: t.currentFunds(),
				Pot:          t.Hand().Pot,
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

func (t *Table) newPotUpdate(pot model.Pot) *PlayerUpdate {
	return &PlayerUpdate{
		Type: PotUpdateT,
		Pot:  pot,
	}
}

func newHandOverUpdate(winners []model.Winner, timeBetweenHands time.Duration, t *Table) *PlayerUpdate {
	return &PlayerUpdate{
		Type:             HandOverUpdateT,
		Winners:          winners,
		TimeBetweenHands: timeBetweenHands,
		CurrentFunds:     t.currentFunds(),
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

// NewFullUpdate creates a FullUpdateT from the player's table
func (p *Player) NewFullUpdate() (*PlayerUpdate, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.table == nil {
		return nil, fmt.Errorf("%s is not at a table", p.GetName())
	}
	return &PlayerUpdate{
		Type:  FullUpdateT,
		Table: p.table.SerializableTable(p),
	}, nil
}

func newTableUpdate(a TableAction) *PlayerUpdate {
	return &PlayerUpdate{
		Type:        TableUpdateT,
		TableAction: a,
	}
}

func newBetUpdate(p *Player, timeRemaining time.Duration, timeElapsed time.Duration) *PlayerUpdate {
	return &PlayerUpdate{
		Type:          BetUpdateT,
		CurrentBetter: p.GetName(),
		CurrentSeat:   p.GetSeat(),
		TimeRemaining: timeRemaining,
		TimeElapsed:   timeElapsed,
	}
}

func (t *Table) newRoundUpdate(a model.RoundAction, p *model.Player, roundDone, roundEndedWithFold, handDone bool) *PlayerUpdate {
	return &PlayerUpdate{
		Type:               RoundUpdateT,
		RoundAction:        a,
		CurrentBetter:      p.Name,
		CurrentSeat:        p.SeatIndex,
		CurrentBets:        t.currentBets(),
		CurrentFunds:       t.currentFunds(),
		Pot:                t.Hand().Pot,
		RoundDone:          roundDone,
		RoundEndedWithFold: roundEndedWithFold,
		HandDone:           handDone,
	}
}

func errorToString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func (t *Table) listenForPlayerActions() {
	for !t.table.RoundDone() && !t.table.HandDone() {
		success := false
		player := t.table.CurrentBetter()
		timeRemaining := t.getTimeToBet()
		var elapsedTime time.Duration
		for !success {
			client := t.players[player.Name]
			a, elapsed := getPlayerAction(timeRemaining, elapsedTime, client, t)
			err := t.table.HandlePlayerAction(player, a, false)
			elapsedTime = elapsed
			if err == nil {
				t.sendPlayerUpdates(t.newRoundUpdate(a, player, t.table.RoundDone(), t.table.Hand.RoundEndedWithFold, t.table.Hand.HandDone))
				success = true
			} else {
				log.Println(err)
			}
			client.responseChan <- RoundActionResponse{errorToString(err)}
		}
		log.Println(player.Name, "made their bet")
	}
	log.Println("Round of betting is done")
	t.table.FinishRound()
	t.sendPlayerUpdates(t.newPotUpdate(t.Hand().Pot))
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
	if p.table == nil {
		return
	}
	wg.Add(1)
	clock := p.table.clock
	go func(p *Player) {
		select {
		case p.TableUpdateChan <- u:
		case <-clock.after(500 * time.Millisecond):
			log.Printf("time out sending to %s's tableUpdateChan", p.playerModel.Name)
		}
		wg.Done()
	}(p)
}

// Returns the selected player action and the elapsed time (not counting any pause)
func getPlayerAction(timeRemaining, elapsedTime time.Duration, player *Player, t *Table) (model.RoundAction, time.Duration) {
	log.Println("Waiting for action from", player.playerModel.Name)
	action := model.RoundAction{ActionType: model.Fold}
	n := t.clock.now()
	for {
		var wg sync.WaitGroup
		afterChan := t.clock.after(timeRemaining - elapsedTime)
		t.sendPlayerUpdates(newBetUpdate(player, timeRemaining, elapsedTime))
		wg.Wait()
		select {
		case <-t.pauseChan:
			elapsedTime += t.clock.now().Sub(n)
			t.updatePaused(true)
			<-t.unpauseChan
			t.updatePaused(false)
			n = t.clock.now()
		case action = <-player.requestChan:
			action = t.sanitizeAction(action, player)
			return action, elapsedTime + t.clock.now().Sub(n)
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
	case PotUpdateT:
		return "PotUpdateT"
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
