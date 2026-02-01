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
		if !table.paused {
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

The frontend will be updated to allow the admin to modify a player's chip count. This will be done using a modal window.

### 3.1 `internal/server/frontend/static/index.html`

This file will be updated to include the HTML for the chip count modal.

#### 3.1.1 Add Chip Count Modal HTML

The following HTML will be added to the `body` of the `index.html` file.

```html
<!-- in internal/server/frontend/static/index.html -->
<div id="chip_count_modal" class="modal hidden">
    <div class="modal-content">
        <span class="close-button">&times;</span>
        <h2>Set Chip Count for <span id="modal_player_name"></span></h2>
        <input type="number" id="new_chip_count_input" />
        <button id="set_chip_count_button">Set</button>
    </div>
</div>
```

#### 3.1.2 Add Modal CSS

The following CSS will be added to the `<style>` section of the `index.html` file.

```css
/* in internal/server/frontend/static/index.html */
.modal {
    position: fixed;
    z-index: 1;
    left: 0;
    top: 0;
    width: 100%;
    height: 100%;
    overflow: auto;
    background-color: rgba(0,0,0,0.4);
}

.modal-content {
    background-color: #fefefe;
    margin: 15% auto;
    padding: 20px;
    border: 1px solid #888;
    width: 80%;
    max-width: 400px;
    border-radius: 8px;
    text-align: center;
}

.close-button {
    color: #aaa;
    float: right;
    font-size: 28px;
    font-weight: bold;
}

.close-button:hover,
.close-button:focus {
    color: black;
    text-decoration: none;
    cursor: pointer;
}
```

### 3.2 `internal/client/web/main.go`

This file contains the client-side logic. We will modify the `onMessage` function to add an onclick handler to each player's name when the table is paused.

#### 3.2.1 Update `Client` struct

A new field will be added to the `Client` struct to hold a reference to the modal element.

```go
// in internal/client/web/main.go
type Client struct {
    // ... existing fields
    chipCountModal js.Value
}
```

#### 3.2.2 Update `makeClient`

The `makeClient` function will be updated to get a reference to the modal element.

```go
// in internal/client/web/main.go
func makeClient() *Client {
    // ... existing code
    return &Client{
        // ... existing fields
        chipCountModal: d.Call("getElementById", "chip_count_modal"),
    }
}
```

#### 3.2.3 Update `onMessage` function

The `onMessage` function will be updated to handle `Pause` and `Unpause` `TableActionResponseT`. When a `Pause` action is received, an `onclick` handler will be added to each player's name. When an `Unpause` action is received, the `onclick` handler will be removed.

```go
// in internal/client/web/main.go
func (c *Client) onMessage(this js.Value, args []js.Value) interface{} {
    // ... existing code
	case gateway.TableActionResponseT:
		if update.TableActionResponse.Err != "" {
			log.Println("error", update.TableActionResponse.TableAction.TableActionType)
			// TODO: display error to user
			return nil
		}
		switch update.TableActionResponse.TableAction.TableActionType {
        // ... existing code
		case tableserver.Unpause:
			c.pauseGameButton.Get("classList").Call("remove", "hidden")
			c.unpauseGameButton.Get("classList").Call("add", "hidden")
            if c.player.Name == c.table.AdminName {
                for i, p := range c.table.Table.Players {
                    if p != nil {
                        seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
                        playerNameEl := seat.Call("querySelector", ".player_name")
                        playerNameEl.Set("onclick", js.Undefined())
                        playerNameEl.Get("style").Set("cursor", "default")
                    }
                }
            }
		case tableserver.Pause:
			c.pauseGameButton.Get("classList").Call("add", "hidden")
			c.unpauseGameButton.Get("classList").Call("remove", "hidden")
            if c.player.Name == c.table.AdminName {
                for i, p := range c.table.Table.Players {
                    if p != nil {
                        seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
                        playerNameEl := seat.Call("querySelector", ".player_name")
                        playerNameEl.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
                            c.showChipCountModal(p.Name)
                            return nil
                        }))
                        playerNameEl.Get("style").Set("cursor", "pointer")
                    }
                }
            }
        // ... existing code
		}
    // ... existing code
}
```

#### 3.2.4 New `showChipCountModal` and `hideChipCountModal` functions

New functions `showChipCountModal` and `hideChipCountModal` will be added to the `Client` struct to control the visibility of the modal.

```go
// in internal/client/web/main.go
func (c *Client) showChipCountModal(playerName string) {
	c.document.Call("getElementById", "modal_player_name").Set("textContent", playerName)
	c.chipCountModal.Get("classList").Call("remove", "hidden")

	closeButton := c.chipCountModal.Call("querySelector", ".close-button")
	closeButton.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.hideChipCountModal()
		return nil
	}))

	setButton := c.chipCountModal.Call("querySelector", "#set_chip_count_button")
	setButton.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.setChipCount(playerName)
		return nil
	}))
}

func (c *Client) hideChipCountModal() {
	c.chipCountModal.Get("classList").Call("add", "hidden")
}
```

#### 3.2.5 Update `setChipCount` function

The `setChipCount` function will be updated to get the new chip amount from the modal's input field and then send a `SetChipCount` action to the server.

```go
// in internal/client/web/main.go
func (c *Client) setChipCount(playerName string) {
	newAmountStr := c.document.Call("getElementById", "new_chip_count_input").Get("value").String()
	c.document.Call("getElementById", "new_chip_count_input").Set("value", "")
	newAmount, err := strconv.Atoi(newAmountStr)
	if err != nil {
		log.Printf("Invalid amount: %s", newAmountStr)
        c.hideChipCountModal()
		return
	}
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.SetChipCount, TableName: c.table.Name, PlayerName: playerName, Amount: newAmount}})
	c.hideChipCountModal()
}
```

#### 3.2.6 Update `handleTableUpdate`

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
