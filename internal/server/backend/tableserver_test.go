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
	c := NewTestClient(t)
	p1 := c.NewPlayer("p1", 1000)
	p2 := c.NewPlayer("p2", 1000)

	c.CreateTable(p1, "test")
	c.JoinTable(p1, "test", false)
	c.JoinTable(p2, "test", false)

	if _, ok := c.ts.tables["test"]; !ok {
		t.Error("expected table to be created")
	}
	if c.ts.tables["test"].adminName != p1.playerModel.Name {
		t.Error("expected creator to be admin")
	}
	if p1.table != c.ts.tables["test"] {
		t.Error("p1's table is not set after joining")
	}
	if p2.table != c.ts.tables["test"] {
		t.Error("p2's table is not set after joining")
	}
	if len(c.ts.tables["test"].players) != 2 {
		t.Errorf("len(c.ts.tables[\"test\"].players) == %d, expected 2", len(c.ts.tables["test"].players))
	}
}

func TestJoinFakeTable(t *testing.T) {
	c := NewTestClient(t)
	p1 := c.NewPlayer("p1", 1000)
	p2 := c.NewPlayer("p2", 1000)

	c.CreateTable(p1, "test")

	c.JoinTable(p2, "fake table", true)
	c.JoinTable(p2, "test", false)
}

func TestSit(t *testing.T) {
	c := NewTestClient(t)
	p1 := c.NewPlayer("p1", 1000)
	p2 := c.NewPlayer("p2", 1000)

	c.CreateTable(p1, "test")
	c.JoinTable(p1, "test", false)
	c.JoinTable(p2, "test", false)
	c.DrainUpdates(p1)
	c.DrainUpdates(p2)

	p1Seat := 0
	c.Sit(p1, p1Seat, false)
	c.Sit(p2, 99, true)
	c.Sit(p2, p1Seat, true)
	c.Sit(p2, 3, false)
}

func TestSimpleHand(t *testing.T) {
	c := NewTestClient(t)
	p1, p2 := c.createTableWithTwoPlayers("test")

	c.StartTable(p2, true)
	c.StartTable(p1, false)

	c.AssertUpdate(p1, NewHandUpdateT)
	u := c.AssertUpdate(p2, NewHandUpdateT)
	if u.Hole == nil {
		t.Errorf("expected player hand provided in NewHandUpdateT, u.Hole == nil")
	}

	table := c.ts.tables["test"]
	if !table.playing {
		t.Error("table should be playing")
	}
	if table.dealer() != p2.playerModel.Name {
		t.Errorf("dealer should be '%v', got '%v'", p2.playerModel.Name, table.dealer())
	}
	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)

	c.SendAction(p2, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	c.SendAction(p1, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p2, RoundUpdateT)
	update := c.AssertUpdate(p1, RoundUpdateT)
	if update.CurrentBetter != p1.playerModel.Name {
		t.Errorf("expected p1, got %s", update.CurrentBetter)
	}

	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
	c.AssertUpdate(p2, DealUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	if len(table.table.Board()) != 3 {
		t.Errorf("expected the flop, len(table.table.Board())==%d", len(table.table.Board()))
	}
	c.Stand(p1)
	if !p1.WantsToStandUp() {
		t.Error("p1 should want to stand up")
	}
	c.Stand(p2)
	c.SendAction(p1, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)

	c.SendAction(p2, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
	c.AssertUpdate(p2, DealUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	if len(table.table.Board()) != 4 {
		t.Errorf("len(table.table.Board())==%d, want 4", len(table.table.Board()))
	}

	c.SendAction(p1, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)

	c.SendAction(p2, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
	c.AssertUpdate(p2, DealUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	if len(table.table.Board()) != 5 {
		t.Errorf("len(table.table.Board())==%d, want 5", len(table.table.Board()))
	}
	c.SendAction(p1, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)

	c.SendAction(p2, model.RoundAction{ActionType: model.Call})
	if !p1.playerModel.WantToStandUp {
		t.Error("p1 should want to stand up")
	}
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, HandOverUpdateT)
	c.AssertUpdate(p2, HandOverUpdateT)
	c.AssertUpdate(p1, TableUpdateT)
	update2 := c.AssertUpdate(p2, TableUpdateT)
	if update2.TableAction.TableActionType != Stand {
		t.Errorf("expected stand update, got %v", update2.TableAction.TableActionType)
	}
	update3 := c.AssertUpdate(p2, StateUpdateT)
	if len(update3.StateUpdate.NowStanding) != 2 {
		t.Errorf("expected two standers, got %d", len(update3.StateUpdate.NowStanding))
	}
	c.AssertUpdate(p2, TableUpdateT)
	c.AssertUpdate(p2, StateUpdateT)
	update4 := <-p2.TableUpdateChan
	if !update4.StateUpdate.PlayStopped {
		t.Error("expected play to stop after standing")
	}
}

func TestAllIn(t *testing.T) {
	c := NewTestClient(t)
	p1, p2 := c.createTableWithTwoPlayers("test")

	c.StartTable(p2, true)
	c.StartTable(p1, false)

	c.AssertUpdate(p1, NewHandUpdateT)
	u := c.AssertUpdate(p2, NewHandUpdateT)
	if u.Hole == nil {
		t.Errorf("expected player hand provided in NewHandUpdateT, u.Hole == nil")
	}
	table := c.ts.tables["test"]
	if !table.playing {
		t.Error("table should be playing")
	}
	if table.dealer() != p2.playerModel.Name {
		t.Errorf("dealer should be '%v', got '%v'", p2.playerModel.Name, table.dealer())
	}
	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.SendAction(p2, model.RoundAction{ActionType: model.AllIn})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	c.SendAction(p1, model.RoundAction{ActionType: model.AllIn})
	c.AssertUpdate(p2, RoundUpdateT)
	update := c.AssertUpdate(p1, RoundUpdateT)
	if update.CurrentBetter != p1.playerModel.Name {
		t.Errorf("expected p1, got %s", update.CurrentBetter)
	}
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
	c.AssertUpdate(p2, DealUpdateT)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
	c.AssertUpdate(p2, DealUpdateT)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
	c.AssertUpdate(p2, DealUpdateT)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, HandOverUpdateT)
	c.AssertUpdate(p2, HandOverUpdateT)
}

func TestTimeoutMidBet(t *testing.T) {
	c := NewTestClient(t)
	p1, p2 := c.createTableWithTwoPlayers("test")
	c.StartTable(p1, false)

	c.AssertUpdate(p1, NewHandUpdateT)
	c.AssertUpdate(p2, NewHandUpdateT)
	table := c.ts.tables["test"]
	if !table.playing {
		t.Error("table should be playing")
	}

	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.SendAction(p2, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)

	c.mockClock.sleep(time.Hour)
	c.AssertUpdate(p2, RoundUpdateT)
	u := c.AssertUpdate(p1, RoundUpdateT)
	if u.RoundAction.ActionType != model.Fold {
		t.Errorf("expected timeout fold, got: %s", u)
	}
	c.AssertRoundActionResponse(p1, false)
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p2, PotUpdateT)
	c.AssertUpdate(p1, HandOverUpdateT)
	c.AssertUpdate(p2, HandOverUpdateT)
	c.AssertUpdate(p1, NewHandUpdateT)
	c.AssertUpdate(p2, NewHandUpdateT)
}

func TestPauseMidBet(t *testing.T) {
	c := NewTestClient(t)
	p1, p2 := c.createTableWithTwoPlayers("test")
	c.StartTable(p1, false)

	c.AssertUpdate(p1, NewHandUpdateT)
	c.AssertUpdate(p2, NewHandUpdateT)
	table := c.ts.tables["test"]
	if !table.playing {
		t.Error("table should be playing")
	}

	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.SendAction(p2, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p1, RoundUpdateT)
	c.AssertUpdate(p2, RoundUpdateT)
	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)

	c.Pause(p1)
	c.AssertUpdate(p1, TableUpdateT)
	c.AssertUpdate(p2, TableUpdateT)

	c.mockClock.sleep(time.Hour)
	c.Unpause(p1)
	c.AssertUpdate(p1, TableUpdateT)
	c.AssertUpdate(p2, TableUpdateT)

	c.AssertUpdate(p1, BetUpdateT)
	c.AssertUpdate(p2, BetUpdateT)
	c.SendAction(p1, model.RoundAction{ActionType: model.Call})
	c.AssertUpdate(p2, RoundUpdateT)
	update := c.AssertUpdate(p1, RoundUpdateT)
	if update.CurrentBetter != p1.playerModel.Name {
		t.Errorf("expected p1, got %s", update.CurrentBetter)
	}
	c.AssertUpdate(p1, PotUpdateT)
	c.AssertUpdate(p1, DealUpdateT)
}

func TestSetChipCount(t *testing.T) {
	c := NewTestClient(t)
	p1, p2 := c.createTableWithTwoPlayers("test")
	newAmount := 1234

	// Fail to set chip count before table is started.
	c.SetChipCount(p1, p2.GetName(), newAmount, true)

	c.StartTable(p1, false)
	c.AssertUpdate(p1, NewHandUpdateT)
	c.AssertUpdate(p2, NewHandUpdateT)
	table := c.ts.tables["test"]
	if !table.playing {
		t.Error("table should be playing")
	}
	c.AssertUpdate(p2, BetUpdateT)
	c.AssertUpdate(p1, BetUpdateT)

	// Fail to set chip count while hand is in progress.
	c.SetChipCount(p1, p2.GetName(), newAmount, true)

	// Have players stand up to stop the hand.
	c.Stand(p1)
	c.Stand(p2)
	// Drain the updates from the stand actions.
	c.DrainUpdates(p1)
	c.DrainUpdates(p2)

	c.Pause(p1)
	checkForUpdate(t, p1, TableUpdateT)
	checkForUpdate(t, p2, TableUpdateT)
	c.AssertTablePaused("test", true)

	// Fail to set chip count as non-admin.
	c.SetChipCount(p2, p1.GetName(), newAmount, true)
	// Fail to set chip count for a user not at the table.
	c.SetChipCount(p1, "fakePlayer", newAmount, true)
	// Fail to set chip count to a negative value.
	c.SetChipCount(p1, p2.GetName(), -1, true)

	// Successfully set the chip count.
	c.SetChipCount(p1, p2.GetName(), newAmount, false)
	update := c.AssertUpdate(p1, TableUpdateT)
	if update.TableAction.TableActionType != SetChipCount {
		t.Errorf("expected a SetChipCount update, got %v", update.TableAction.TableActionType)
	}
	if update.TableAction.Amount != newAmount {
		t.Errorf("expected amount to be %d, got %d", newAmount, update.TableAction.Amount)
	}
	if update.TableAction.PlayerName != p2.GetName() {
		t.Errorf("expected player name to be %s, got %s", p2.GetName(), update.TableAction.PlayerName)
	}
	update = c.AssertUpdate(p2, TableUpdateT)
	if update.TableAction.TableActionType != SetChipCount {
		t.Errorf("expected a SetChipCount update, got %v", update.TableAction.TableActionType)
	}
	if p2.playerModel.Funds != newAmount {
		t.Errorf("expected player funds to be %d, got %d", newAmount, p2.playerModel.Funds)
	}
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
		t.Fatal("Timeout in checkRoundResponse")
		return nil
	}
}
