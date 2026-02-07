package tableserver

import (
	"sync"
	"testing"
	"time"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

type TestClient struct {
	t         *testing.T
	ts        *TableServer
	mockClock *mockTime
	players   map[string]*Player
	mutex     sync.Mutex
}

func NewTestClient(t *testing.T) *TestClient {
	mockClock := &mockTime{}
	ts := NewTableServerWithTime(mockClock)
	go ts.Serve()
	return &TestClient{
		t:         t,
		ts:        ts,
		mockClock: mockClock,
		players:   make(map[string]*Player),
	}
}

func (c *TestClient) NewPlayer(name string, funds int) *Player {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	p := NewPlayer(name)
	p.playerModel.Funds = funds
	c.players[name] = p
	return p
}

func (c *TestClient) CreateTable(p *Player, tableName string) {
	c.ts.SendTableAction(CreateTableAction(tableName, p))
	c.AssertTableActionResponse(p, Create, false)
}

func (c *TestClient) JoinTable(p *Player, tableName string, expectErr bool) {
	c.ts.SendTableAction(JoinTableAction(tableName, p))
	c.AssertTableActionResponse(p, Join, expectErr)
	if !expectErr {
		c.ForAllPlayersInTable(p.table, func(player *Player) {
			c.AssertUpdate(player, TableUpdateT)
		})
	}
}

func (c *TestClient) Sit(p *Player, seat int, expectErr bool) {
	c.ts.SendTableAction(SitTableAction(p.table.name, p, seat))
	c.AssertTableActionResponse(p, Sit, expectErr)
	if !expectErr {
		c.ForAllPlayersInTable(p.table, func(player *Player) {
			c.AssertUpdate(player, TableUpdateT)
		})
	}
}

func (c *TestClient) StartTable(p *Player, expectErr bool) {
	c.ts.SendTableAction(StartTableAction(p.table.name, p))
	c.AssertTableActionResponse(p, Start, expectErr)
	if !expectErr {
		c.ForAllPlayersInTable(p.table, func(player *Player) {
			c.AssertUpdate(player, TableUpdateT)
		})
	}
}

func (c *TestClient) createTableWithTwoPlayers(tableName string) (*Player, *Player) {
	p1 := c.NewPlayer("p1", 1000)
	p2 := c.NewPlayer("p2", 1000)
	c.CreateTable(p1, tableName)
	c.JoinTable(p1, tableName, false)
	c.JoinTable(p2, tableName, false)
	c.DrainUpdates(p1)
	c.DrainUpdates(p2)
	c.Sit(p1, 2, false)
	c.Sit(p2, 1, false)
	return p1, p2
}

func (c *TestClient) Stand(p *Player) {
	c.ts.SendTableAction(StandTableAction(p.table.name, p))
	c.AssertTableActionResponse(p, Stand, false)
}

func (c *TestClient) Pause(p *Player) {
	c.ts.SendTableAction(PauseTableAction(p.table.name, p))
	c.AssertTableActionResponse(p, Pause, false)
}

func (c *TestClient) Unpause(p *Player) {
	c.ts.SendTableAction(UnpauseTableAction(p.table.name, p))
	c.AssertTableActionResponse(p, Unpause, false)
}

func (c *TestClient) SetChipCount(p *Player, playerName string, amount int, expectErr bool) {
	c.ts.SendTableAction(SetChipCountTableAction(p.table.name, p, playerName, amount))
	c.AssertTableActionResponse(p, SetChipCount, expectErr)
}

func (c *TestClient) AssertTablePaused(tableName string, expectedPausedState bool) {
	c.t.Helper()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	table := c.ts.tables[tableName]
	// Retry checking the paused state for a short duration to account for asynchronous updates.
	for i := 0; i < 10; i++ {
		if table.paused == expectedPausedState {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if table.paused != expectedPausedState {
		c.t.Errorf("expected table %s paused state to be %v, got %v after retries", tableName, expectedPausedState, table.paused)
	}
}

func (c *TestClient) SendAction(p *Player, action model.RoundAction) {
	sendRoundActionWithTimeout(c.t, p, action)
	c.AssertRoundActionResponse(p, false)
}

func (c *TestClient) AssertTableActionResponse(p *Player, actionType TableActionType, expectErr bool) {
	select {
	case resp := <-p.TableResponseChan():
		if (resp.Err != "") != expectErr {
			c.t.Errorf("player %s: expected error %v, got %v for action %s", p.GetName(), expectErr, resp.Err, actionType)
		}
	case <-time.After(time.Second):
		c.t.Fatalf("player %s: timeout waiting for table action response for action %s", p.GetName(), actionType)
	}
}

func (c *TestClient) AssertRoundActionResponse(p *Player, expectErr bool) {
	select {
	case resp := <-p.RoundResponseChan():
		if (resp.Err != "") != expectErr {
			c.t.Errorf("player %s: expected error %v, got %v for round action", p.GetName(), expectErr, resp.Err)
		}
	case <-time.After(time.Second):
		c.t.Fatalf("player %s: timeout waiting for round action response", p.GetName())
	}
}

func (c *TestClient) AssertUpdate(p *Player, updateType PlayerUpdateType) *PlayerUpdate {
	c.t.Helper()
	select {
	case update := <-p.TableUpdateChan:
		if update.Type != updateType {
			c.t.Errorf("player %s: expected update of type %s, got %s", p.GetName(), updateType, update.Type)
		}
		return update
	case <-time.After(time.Second):
		c.t.Fatalf("player %s: timeout waiting for update of type %s", p.GetName(), updateType)
		return nil
	}
}

func (c *TestClient) DrainUpdates(p *Player) {
	for {
		select {
		case <-p.TableUpdateChan:
		default:
			return
		}
	}
}

func (c *TestClient) ForAllPlayersInTable(t *Table, fn func(*Player)) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if t == nil {
		return
	}
	for _, p := range t.players {
		fn(p)
	}
}
