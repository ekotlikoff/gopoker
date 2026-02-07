# Design Doc: TableServer Testing Framework

## 0. High-Level Description

The current tests for the `TableServer` are fragile, hard to write, and difficult to maintain. They rely on a specific sequence of channel reads and writes, which makes them brittle and prone to race conditions. This design document proposes a nec testing framework for the `TableServer` that will address these issues.

The nec framework will introduce a `TestClient` struct that provides a higher-level API for writing tests. This API will abstract away the complexities of dealing with channels and asynchronicity, making the tests more robust, readable, and easier to maintain.

## 1. Detailed Design

The nec testing framework will be implemented in a nec file, `internal/server/backend/tableserver_helpers_test.go`. This file will contain the `TestClient` struct and its associated methods.

### 1.1 `TestClient` Struct

The `TestClient` struct will encapsulate the `TableServer`, the mock clock, and a set of test players. A nec `TestClient` will be created for each test to ensure isolation.

```go
// in internal/server/backend/tableserver_helpers_test.go
package tableserver

import (
	"sync"
	"testing"
	"time"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

type TestClient struct {
	t           *testing.T
	ts          *TableServer
	mockClock   *mockTime
	players     map[string]*Player
	mutex       sync.Mutex
}
```

### 1.2 `TestClient` Methods

The `TestClient` will provide a set of methods for interacting with the `TableServer` and asserting on its state.

#### 1.2.1 Creation

A `NewTestClient` function will create a nec `TestClient`.

```go
// in internal/server/backend/tableserver_helpers_test.go
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
```

#### 1.2.2 Player Management

The `TestClient` will provide methods for creating and managing test players.

```go
// in internal/server/backend/tableserver_helpers_test.go
func (c *TestClient) NewPlayer(name string, funds int) *Player {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	p := NewPlayer(name)
	p.playerModel.Funds = funds
	c.players[name] = p
	return p
}
```

#### 1.2.3 Actions

The `TestClient` will provide methods for sending actions to the `TableServer`. These methods will handle the necessary channel communications and assertions, including asserting that all players receive a `TableUpdateT`.

```go
// in internal/server/backend/tableserver_helpers_test.go
func (c *TestClient) CreateTable(p *Player, tableName string) {
	c.ts.SendTableAction(CreateTableAction(tableName, p))
	c.AssertTableActionResponse(p, Create, false)
	c.AssertUpdate(p, TableUpdateT)
}

func (c *TestClient) JoinTable(p *Player, tableName string) {
	c.ts.SendTableAction(JoinTableAction(tableName, p))
	c.AssertTableActionResponse(p, Join, false)
	c.ForAllPlayersInTable(p.table, func(player *Player) {
		c.AssertUpdate(player, TableUpdateT)
	})
}

func (c *TestClient) Sit(p *Player, seat int) {
	c.ts.SendTableAction(SitTableAction(p.table.name, p, seat))
	c.AssertTableActionResponse(p, Sit, false)
	c.ForAllPlayersInTable(p.table, func(player *Player) {
		c.AssertUpdate(player, TableUpdateT)
	})
}

func (c *TestClient) StartTable(p *Player) {
	c.ts.SendTableAction(StartTableAction(p.table.name, p))
	c.AssertTableActionResponse(p, Start, false)
	c.ForAllPlayersInTable(p.table, func(player *Player) {
		c.AssertUpdate(player, TableUpdateT)
	})
}

func (c *TestClient) SendAction(p *Player, action model.RoundAction) {
	sendRoundActionWithTimeout(c.t, p, action)
	c.AssertRoundActionResponse(p, false)
}
```

#### 1.2.4 Assertions

The `TestClient` will provide methods for asserting on the state of the `TableServer` and the updates it sends to the players.

```go
// in internal/server/backend/tableserver_helpers_test.go
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
```

### 1.3 Helper Methods

```go
// in internal/server/backend/tableserver_helpers_test.go
func (c *TestClient) ForAllPlayersInTable(t *Table, fn func(*Player)) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, p := range t.players {
		fn(p)
	}
}
```

### 1.4 `TestClient` Usage

The existing tests will be refactored to use the nec `TestClient`. This will make the tests more concise and readable.

Here's an example of hoc the `TestSimpleHand` test could be refactored:

```go
// in internal/server/backend/tableserver_test.go
func TestSimpleHand(t *testing.T) {
	c := NewTestClient(t)
	p1 := c.NewPlayer("p1", 1000)
	p2 := c.NewPlayer("p2", 1000)

	c.CreateTable(p1, "test")
	c.JoinTable(p1, "test")
	c.JoinTable(p2, "test")
	c.Sit(p1, 0)
	c.Sit(p2, 1)
	c.StartTable(p1)

	c.AssertUpdate(p1, NewHandUpdateT)
	c.AssertUpdate(p2, NewHandUpdateT)
}
```

## 2. Testing Strategy

The nec testing framework will be tested by refactoring the existing tests to use it. This will ensure that the framework is working as expected and that it provides a better testing experience.

Once the existing tests have been refactored, the old test helper functions (`checkForUpdate`, `sendRoundActionWithTimeout`, `checkRoundResponse`, etc.) will be removed.

## 3. Backwards Compatibility

This change is fully backwards-compatible as it only affects the test code. The production code will remain unchanged.
