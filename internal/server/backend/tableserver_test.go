package tableserver

import (
	"sync"
	"testing"
	"time"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

type mockTime struct {
	t           time.Time
	tickers     []chan time.Time
	tickerTimes []time.Time
	mutex       sync.RWMutex
}

func (m *mockTime) now() time.Time { m.mutex.RLock(); defer m.mutex.RUnlock(); return m.t }
func (m *mockTime) after(d time.Duration) <-chan time.Time {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.tickerTimes = append(m.tickerTimes, m.t.Add(d))
	ticker := make(chan time.Time, 5)
	m.tickers = append(m.tickers, ticker)
	return ticker
}
func (m *mockTime) sleep(d time.Duration) {
	m.stepTime(d)
}
func (m *mockTime) stepTime(d time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.t = m.t.Add(d)
	for i, t := range m.tickerTimes {
		if m.t.After(t) {
			m.tickers[i] <- m.t
		}
	}
}

func TestCreateAndJoin(t *testing.T) {
	tableName := "test table"
	ts := NewTableServerWithTime(&mockTime{})
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ts.SendTableAction(CreateTableAction(tableName, p1))
	if p1.GetTableResponse().Err != "" {
		t.Error("expected to create table successfully")
	}
	if _, ok := ts.tables[tableName]; !ok {
		t.Error("expected table to be created")
	}
	if ts.tables[tableName].adminName != p1.playerModel.Name {
		t.Error("expected creator to be admin")
	}
	ts.SendTableAction(JoinTableAction(tableName, p1))
	if p1.GetTableResponse().Err != "" {
		t.Error("expected to join table successfully")
	}
	if p1.table != ts.tables[tableName] {
		t.Error("p1's table is not set after joining")
	}
	ts.SendTableAction(JoinTableAction(tableName, p2))
	if p2.GetTableResponse().Err != "" {
		t.Error("expected to join table successfully")
	}
	if p2.table != ts.tables[tableName] {
		t.Error("p2's table is not set after joining")
	}
	if len(ts.tables[tableName].players) != 2 {
		t.Errorf("len(ts.tables[tableName].players) == %d, expected 2", len(ts.tables[tableName].players))
	}
}

func TestJoinFakeTable(t *testing.T) {
	tableName := "test table"
	ts := NewTableServerWithTime(&mockTime{})
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ts.SendTableAction(CreateTableAction(tableName, p1))
	if p1.GetTableResponse().Err != "" {
		t.Error("expected to create table successfully")
	}
	ts.SendTableAction(JoinTableAction("fake table", p2))
	if p2.GetTableResponse().Err == "" {
		t.Error("expected to fail to join table")
	}
	ts.SendTableAction(JoinTableAction(tableName, p2))
	if p2.GetTableResponse().Err != "" {
		t.Error("expected to join table successfully")
	}
}

func TestSit(t *testing.T) {
	tableName := "test table"
	ts := NewTableServerWithTime(&mockTime{})
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	p1.playerModel.Funds = 1000
	p2.playerModel.Funds = 1000
	ts.SendTableAction(CreateTableAction(tableName, p1))
	p1.GetTableResponse()
	ts.SendTableAction(JoinTableAction(tableName, p2))
	p2.GetTableResponse()
	p1Seat := 0
	ts.SendTableAction(SitTableAction(tableName, p1, p1Seat))
	if err := p1.GetTableResponse().Err; err != "" {
		t.Errorf("expected to sit at table successfully, failed with error: %s", err)
	}
	seat := 99
	ts.SendTableAction(SitTableAction(tableName, p2, seat))
	if p2.GetTableResponse().Err == "" {
		t.Errorf("expected not to sit at seat %d successfully", seat)
	}
	ts.SendTableAction(SitTableAction(tableName, p2, p1Seat))
	if p2.GetTableResponse().Err == "" {
		t.Errorf("expected not to sit at seat %d successfully", seat)
	}
	ts.SendTableAction(SitTableAction(tableName, p2, 3))
	if err := p2.GetTableResponse().Err; err != "" {
		t.Errorf("expected to sit successfully, failed with error: %s", err)
	}
}

func consumeTableUpdates(ps ...*Player) {
	for _, p := range ps {
		<-p.TableUpdateChan
	}
}

func createTableWithTwoPlayers(tableName string) (*TableServer, *Player, *Player) {
	ts := NewTableServerWithTime(&mockTime{})
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ps := []*Player{p1, p2}
	p1.playerModel.Funds = 1000
	p2.playerModel.Funds = 1000
	ts.SendTableAction(CreateTableAction(tableName, p1))
	p1.GetTableResponse()
	ts.SendTableAction(JoinTableAction(tableName, p1))
	consumeTableUpdates(p1)
	p1.GetTableResponse()
	ts.SendTableAction(JoinTableAction(tableName, p2))
	consumeTableUpdates(ps...)
	p2.GetTableResponse()
	ts.SendTableAction(SitTableAction(tableName, p1, 2))
	consumeTableUpdates(ps...)
	p1.GetTableResponse()
	ts.SendTableAction(SitTableAction(tableName, p2, 1))
	consumeTableUpdates(ps...)
	p2.GetTableResponse()
	return ts, p1, p2
}

func TestSimpleHand(t *testing.T) {
	tableName := "test table"
	ts, p1, p2 := createTableWithTwoPlayers(tableName)
	ts.SendTableAction(StartTableAction(tableName, p2))
	if r := p2.GetTableResponse(); r.Err == "" {
		t.Error("only the admin should be able to start the table")
	}
	ts.SendTableAction(StartTableAction(tableName, p1))
	if r := p1.GetTableResponse(); r.Err != "" {
		t.Error("the admin should be able to start the table")
	}
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	checkForUpdate(t, p1, NewHandUpdateT)
	u := checkForUpdate(t, p2, NewHandUpdateT)
	if u.Hole == nil {
		t.Errorf("expected player hand provided in NewHandUpdateT, u.Hole == nil")
	}
	table := ts.tables[tableName]
	if !table.playing {
		t.Error("table should be playing")
	}
	if table.dealer() != p2.playerModel.Name {
		t.Errorf("dealer should be '%v', got '%v'", p2.playerModel.Name, table.dealer())
	}
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.Call})
	checkRoundResponse(t, p2, false)
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	sendRoundActionWithTimeout(t, p1, model.RoundAction{ActionType: model.Call})
	checkRoundResponse(t, p1, false)
	checkForUpdate(t, p2, RoundUpdateT)
	update := checkForUpdate(t, p1, RoundUpdateT)
	if update.CurrentBetter != p1.playerModel.Name {
		t.Errorf("expected p1, got %s", update.CurrentBetter)
	}
	checkForUpdate(t, p1, DealUpdateT)
	checkForUpdate(t, p2, DealUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	if len(table.table.Board()) != 3 {
		t.Errorf("expected the flop, len(table.table.Board())==%d", len(table.table.Board()))
	}
	ts.SendTableAction(StandTableAction(tableName, p1))
	if r := p1.GetTableResponse(); r.Err != "" {
		t.Errorf("expected a successful stand, got error: %s", r.Err)
	}
	if !p1.playerModel.WantToStandUp {
		t.Error("p1 should want to stand up")
	}
	ts.SendTableAction(StandTableAction(tableName, p2))
	p2.GetTableResponse()
	sendRoundActionWithTimeout(t, p1, model.RoundAction{ActionType: model.Call})
	if r := p1.GetRoundResponse(); r.Err != "" {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	// // TODO add timeouts for the server's SendRoundResponse
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.Call})
	checkRoundResponse(t, p2, false)
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, DealUpdateT)
	checkForUpdate(t, p2, DealUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	if len(table.table.Board()) != 4 {
		t.Errorf("len(table.table.Board())==%d, want 4", len(table.table.Board()))
	}
	sendRoundActionWithTimeout(t, p1, model.RoundAction{ActionType: model.Call})
	if r := p1.GetRoundResponse(); r.Err != "" {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.Call})
	checkRoundResponse(t, p2, false)
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, DealUpdateT)
	checkForUpdate(t, p2, DealUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	if len(table.table.Board()) != 5 {
		t.Errorf("len(table.table.Board())==%d, want 5", len(table.table.Board()))
	}
	sendRoundActionWithTimeout(t, p1, model.RoundAction{ActionType: model.Call})
	if r := p1.GetRoundResponse(); r.Err != "" {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.Call})
	if !p1.playerModel.WantToStandUp {
		t.Error("p1 should want to stand up")
	}
	checkRoundResponse(t, p2, false)
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, HandOverUpdateT)
	checkForUpdate(t, p2, HandOverUpdateT)
	checkForUpdate(t, p1, TableUpdateT)
	update = checkForUpdate(t, p2, TableUpdateT)
	if update.TableAction.TableActionType != Stand {
		t.Errorf("expected stand update, got %v", update.TableAction.TableActionType)
	}
	update = checkForUpdate(t, p2, StateUpdateT)
	if len(update.StateUpdate.NowStanding) != 2 {
		t.Errorf("expected two standers, got %d", len(update.StateUpdate.NowStanding))
	}
	checkForUpdate(t, p2, TableUpdateT)
	checkForUpdate(t, p2, StateUpdateT)
	update = <-p2.TableUpdateChan
	if !update.StateUpdate.PlayStopped {
		t.Error("expected play to stop after standing")
	}
}

func TestAllIn(t *testing.T) {
	tableName := "test table"
	ts, p1, p2 := createTableWithTwoPlayers(tableName)
	ts.SendTableAction(StartTableAction(tableName, p2))
	if r := p2.GetTableResponse(); r.Err == "" {
		t.Error("only the admin should be able to start the table")
	}
	ts.SendTableAction(StartTableAction(tableName, p1))
	if r := p1.GetTableResponse(); r.Err != "" {
		t.Error("the admin should be able to start the table")
	}
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	checkForUpdate(t, p1, NewHandUpdateT)
	u := checkForUpdate(t, p2, NewHandUpdateT)
	if u.Hole == nil {
		t.Errorf("expected player hand provided in NewHandUpdateT, u.Hole == nil")
	}
	table := ts.tables[tableName]
	if !table.playing {
		t.Error("table should be playing")
	}
	if table.dealer() != p2.playerModel.Name {
		t.Errorf("dealer should be '%v', got '%v'", p2.playerModel.Name, table.dealer())
	}
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.AllIn})
	checkRoundResponse(t, p2, false)
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	sendRoundActionWithTimeout(t, p1, model.RoundAction{ActionType: model.AllIn})
	checkRoundResponse(t, p1, false)
	checkForUpdate(t, p2, RoundUpdateT)
	update := checkForUpdate(t, p1, RoundUpdateT)
	if update.CurrentBetter != p1.playerModel.Name {
		t.Errorf("expected p1, got %s", update.CurrentBetter)
	}
	checkForUpdate(t, p1, DealUpdateT)
	checkForUpdate(t, p2, DealUpdateT)
	checkForUpdate(t, p1, DealUpdateT)
	checkForUpdate(t, p2, DealUpdateT)
	checkForUpdate(t, p1, DealUpdateT)
	checkForUpdate(t, p2, DealUpdateT)
	checkForUpdate(t, p1, HandOverUpdateT)
	checkForUpdate(t, p2, HandOverUpdateT)
}

func TestTimeoutMidBet(t *testing.T) {
	tableName := "test table"
	ts, p1, p2 := createTableWithTwoPlayers(tableName)
	ts.SendTableAction(StartTableAction(tableName, p1))
	p1.GetTableResponse()
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	checkForUpdate(t, p1, NewHandUpdateT)
	checkForUpdate(t, p2, NewHandUpdateT)
	table := ts.tables[tableName]
	if !table.playing {
		t.Error("table should be playing")
	}
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.Call})
	checkRoundResponse(t, p2, false)
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	ts.clock.sleep(time.Hour)
	checkForUpdate(t, p2, RoundUpdateT)
	u := checkForUpdate(t, p1, RoundUpdateT)
	if u.RoundAction.ActionType != model.Fold {
		t.Errorf("expected timeout fold, got: %s", u)
	}
	checkRoundResponse(t, p1, false)
	checkForUpdate(t, p1, HandOverUpdateT)
	checkForUpdate(t, p2, HandOverUpdateT)
	checkForUpdate(t, p1, NewHandUpdateT)
	checkForUpdate(t, p2, NewHandUpdateT)
}

func TestPauseMidBet(t *testing.T) {
	tableName := "test table"
	ts, p1, p2 := createTableWithTwoPlayers(tableName)
	ts.SendTableAction(StartTableAction(tableName, p1))
	p1.GetTableResponse()
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	checkForUpdate(t, p1, NewHandUpdateT)
	checkForUpdate(t, p2, NewHandUpdateT)
	table := ts.tables[tableName]
	if !table.playing {
		t.Error("table should be playing")
	}
	checkForUpdate(t, p2, BetUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	sendRoundActionWithTimeout(t, p2, model.RoundAction{ActionType: model.Call})
	if r := p2.GetRoundResponse(); r.Err != "" {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	checkForUpdate(t, p1, RoundUpdateT)
	checkForUpdate(t, p2, RoundUpdateT)
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	ts.SendTableAction(PauseTableAction(tableName, p1))
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	if p1.GetTableResponse().Err != "" {
		t.Error("expected to pause table successfully")
	}
	ts.clock.sleep(time.Hour)
	ts.SendTableAction(UnpauseTableAction(tableName, p1))
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	if p1.GetTableResponse().Err != "" {
		t.Error("expected to pause table successfully")
	}
	checkForUpdate(t, p1, BetUpdateT)
	checkForUpdate(t, p2, BetUpdateT)
	sendRoundActionWithTimeout(t, p1, model.RoundAction{ActionType: model.Call})
	if r := p1.GetRoundResponse(); r.Err != "" {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	checkForUpdate(t, p2, RoundUpdateT)
	update := checkForUpdate(t, p1, RoundUpdateT)
	if update.CurrentBetter != p1.playerModel.Name {
		t.Errorf("expected p1, got %s", update.CurrentBetter)
	}
	checkForUpdate(t, p1, DealUpdateT)
}

func checkForUpdate(t *testing.T, p *Player, want PlayerUpdateType) *PlayerUpdate {
	t.Helper()
	select {
	case u := <-p.TableUpdateChan:
		if u.Type != want {
			t.Errorf("want %v got %v", want, u.Type)
		}
		return u
	case <-time.After(time.Second):
		t.Fail()
		return nil
	}
}

func sendRoundActionWithTimeout(t *testing.T, p *Player, ra model.RoundAction) {
	t.Helper()
	select {
	case p.requestChan <- ra:
		return
	case <-time.After(time.Second):
		t.Fail()
		return
	}
}

func checkRoundResponse(t *testing.T, p *Player, expectErr bool) *RoundActionResponse {
	t.Helper()
	select {
	case r := <-p.responseChan:
		if (expectErr && r.Err == "") || (!expectErr && r.Err != "") {
			t.Errorf("want err: %v, got %v", expectErr, r.Err)
		}
		return &r
	case <-time.After(time.Second):
		t.Fail()
		return nil
	}
}
