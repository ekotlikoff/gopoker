# Design Doc: Admin Chip Count Modification

## 0. High-Level Description

This feature will allow the administrator of a poker table to modify the chip counts of any player at the table. This action will only be permissible when the table is in a "paused" state. When the admin modifies a player's chip count, a `TableUpdate` will be broadcast to all players at the table, informing them of the change. This functionality is useful for managing games, correcting errors, or for friendly games where players might buy-in or cash-out between hands.

## 1. Detailed Design

The implementation of this feature will involve changes to the following files:

*   `internal/server/backend/tableserver.go`
*   `internal/server/backend/player.go`
*   `internal/model/table/table.go`

### 1.1 `internal/server/backend/tableserver.go`

This file contains the core logic for the table server, which manages the state of the tables and the players. We will need to add a new `TableActionType` and a new `TableAction` to handle the chip count modification.

#### 1.1.1 New `TableActionType`

We will add a new `TableActionType` called `SetChipCount` to the `TableActionType` enum.

```go
// in internal/server/backend/tableserver.go
const (
	// ... existing action types
	SetChipCount TableActionType = "setChipCount"
)
```

#### 1.1.2 New `TableAction` struct

We will add a new field to the `TableAction` struct to carry the necessary information for the `SetChipCount` action. This will include the name of the player whose chips are being modified and the new chip amount.

```go
// in internal/server/backend/tableserver.go
type TableAction struct {
	// ... existing fields
	PlayerName string `json:"playerName,omitempty"`
	Amount     int    `json:"amount,omitempty"`
}
```

#### 1.1.3 New `SetChipCountTableAction` helper function

We will add a new helper function to create a `SetChipCount` `TableAction`.

```go
// in internal/server/backend/tableserver.go
func SetChipCountTableAction(tableName string, from *Player, playerName string, amount int) *PlayerAction {
	return &PlayerAction{
		From: from,
		Action: &TableAction{
			TableActionType: SetChipCount,
			TableName:       tableName,
			PlayerName:      playerName,
			Amount:          amount,
		},
	}
}
```

#### 1.1.4 Update the `handleTableAction` function

We will update the `handleTableAction` function in `tableserver.go` to handle the new `SetChipCount` action. The logic will be as follows:

1.  Check if the table exists.
2.  Check if the table is paused. If not, return an error.
3.  Check if the `from` player is the admin of the table. If not, return an error.
4.  Find the player whose chip count is to be modified.
5.  Validate that the `Amount` is a non-negative integer.
6.  Update the player's chip count.
7.  Broadcast a `TableUpdate` to all players at the table, informing them of the change.

```go
// in internal/server/backend/tableserver.go
func (ts *TableServer) handleTableAction(action *PlayerAction) {
	// ... existing code
	case SetChipCount:
		table, ok := ts.tables[a.TableName]
		if !ok {
			action.From.SendTableResponse(
				"", fmt.Sprintf("Table %s not found.", a.TableName))
			return
		}
		if table.playing {
			action.From.SendTableResponse(
				"", "Cannot set chip count while table is playing.")
			return
		}
		if table.adminName != action.From.playerModel.Name {
			action.From.SendTableResponse(
				"", "Only the admin can set chip counts.")
			return
		}
		if a.Amount < 0 {
			action.From.SendTableResponse(
				"", "Chip count cannot be negative.")
			return
		}
		var targetPlayer *Player
		for _, p := range table.players {
			if p.playerModel.Name == a.PlayerName {
				targetPlayer = p
				break
			}
		}
		if targetPlayer == nil {
			action.From.SendTableResponse(
				"", fmt.Sprintf("Player %s not found.", a.PlayerName))
			return
		}
		targetPlayer.playerModel.Funds = a.Amount
		table.broadcast(&PlayerUpdate{
			Type: TableUpdateT,
			TableAction: &TableAction{
				TableActionType: SetChipCount,
				From:            action.From.playerModel.Name,
				PlayerName:      a.PlayerName,
				Amount:          a.Amount,
			},
		})
	// ... existing code
}
```

#### 1.1.5 New `broadcast` method on `Table` with timeout

We will add a new method to the `Table` struct in `tableserver.go` to broadcast updates to all players at the table. This function will include a timeout to prevent blocking if a player's channel is full.

```go
// in internal/server/backend/tableserver.go
func (t *Table) broadcast(update *PlayerUpdate) {
	for _, p := range t.players {
		select {
		case p.TableUpdateChan <- update:
		case <-time.After(1 * time.Second):
			// Log the error and continue to the next player.
			// This prevents a slow client from blocking the entire table.
			log.Printf("Warning: player %s update channel is full. Discarding update.", p.playerModel.Name)
		}
	}
}
```

### 1.2 `internal/server/backend/player.go`

This file defines the `Player` struct and its associated methods. No changes are required in this file.

### 1.3 `internal/model/table/table.go`

This file defines the `Table` struct and its associated methods. No changes are required in this file.

## 2. Testing Strategy

The testing strategy will consist of unit tests and integration tests.

### 2.1 Unit Tests

We will add a new unit test to `internal/server/backend/tableserver_test.go` to test the `SetChipCount` action. This test will cover the following scenarios:

*   A non-admin user attempts to set a player's chip count.
*   An admin attempts to set a player's chip count while the table is playing.
*   An admin attempts to set a player's chip count to a negative value.
*   An admin attempts to set the chip count of a non-existent player.
*   An admin successfully sets a player's chip count.

### 2.2 Integration Tests

We will create a new integration test that simulates a real-world scenario. The test will:

1.  Create a table with three players.
2.  Start the game and play a few hands.
3.  Pause the game.
4.  Have the admin change the chip count of one of the players.
5.  Unpause the game and continue playing.
6.  Verify that the chip count of the modified player is correct.
7.  Verify that all players receive the `TableUpdate` notification.

This comprehensive testing strategy will ensure that the new feature is working as expected and does not introduce any regressions.

## 3. Frontend Changes

The frontend will be updated to allow the admin to modify a player's chip count.

### 3.1 `internal/client/web/main.go`

This file contains the client-side logic. We will modify the `renderFullTable` function to add an onclick handler to each player's name.

#### 3.1.1 Update `renderFullTable`

The `renderFullTable` function will be updated to add a click event listener to each player's name element. This event listener will only be added if the current player is the admin and the table is paused. When a player's name is clicked, a new `setChipCount` function will be called.

```go
// in internal/client/web/main.go
func (c *Client) renderFullTable(table tableserver.SerializableTable) {
	// ... existing code
	if player != nil {
		playerName.Set("textContent", player.Name)
		if c.player.Name == table.AdminName && table.Paused {
			playerName.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				c.setChipCount(player.Name)
				return nil
			}))
			playerName.Get("style").Set("cursor", "pointer")
		}
// ... existing code
}
```

#### 3.1.2 New `setChipCount` function

A new function `setChipCount` will be added to the `Client` struct. This function will prompt the admin for a new chip count and then send a `SetChipCount` action to the server.

```go
// in internal/client/web/main.go
func (c *Client) setChipCount(playerName string) {
	newAmountStr := js.Global().Call("prompt", fmt.Sprintf("Enter new chip count for %s", playerName), "").String()
	if newAmountStr == "" {
		return
	}
	newAmount, err := strconv.Atoi(newAmountStr)
	if err != nil {
		log.Printf("Invalid amount: %s", newAmountStr)
		return
	}
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.SetChipCount, TableName: c.table.Name, PlayerName: playerName, Amount: newAmount}})
}
```

#### 3.1.3 Update `handleTableUpdate`

The `handleTableUpdate` function will be updated to handle the `SetChipCount` action. When a `SetChipCount` action is received, the player's funds will be updated in the UI.

```go
// in internal/client/web/main.go
func (c *Client) handleTableUpdate(action tableserver.TableAction) {
	switch action.TableActionType {
// ... existing code
	case tableserver.SetChipCount:
		for i, p := range c.table.Table.Players {
			if p != nil && p.Name == action.PlayerName {
				c.table.Table.Players[i].Funds = action.Amount
				seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
				chipsDiv := seat.Call("querySelector", ".player_funds")
				if action.Amount > 0 {
					chipsDiv.Set("textContent", fmt.Sprintf("$%d", action.Amount))
				} else {
					chipsDiv.Set("textContent", "")
				}
				break
			}
		}
// ... existing code
	}
}
```
