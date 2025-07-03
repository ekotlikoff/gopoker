//go:build wasm && js && webclient

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"syscall/js"

	"github.com/chehsunliu/poker"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
	tableserver "github.com/ekotlikoff/gopoker/internal/server/backend"
	gateway "github.com/ekotlikoff/gopoker/internal/server/frontend"
)

type Client struct {
	document js.Value
	client   *http.Client
	conn     js.Value // WebSocket connection
	player   *model.Player
	table    tableserver.SerializableTable
	tables   []model.TableSummary
}

func main() {
	done := make(chan struct{})

	jar, _ := cookiejar.New(&cookiejar.Options{})
	c := &Client{
		document: js.Global().Get("document"),
		client:   &http.Client{Jar: jar},
	}

	c.initLobby()

	<-done
}

func (c *Client) initLobby() {
	go func() {
		if c.getSession() {
			c.showLobby()
		} else {
			c.showLogin()
		}
		c.getTables()
		c.renderTables()
	}()
	js.Global().Get("document").Call("getElementById", "create_table_button").Set("onclick", js.FuncOf(c.createTable))
}

func (c *Client) getSession() bool {
	resp, err := c.client.Get("session")
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	defer resp.Body.Close()

	var session gateway.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return false
	}

	c.player = model.NewPlayer(session.Credentials.Username)
	return true
}

func (c *Client) showLogin() {
	// Similar to gochess, we can add a login form if needed.
	// For now, we'll just show the lobby and assume a user.
	c.showLobby()
}

func (c *Client) showLobby() {
	c.document.Call("getElementById", "lobby_page").Set("className", "")
	c.document.Call("getElementById", "table_page").Set("className", "hidden")
}

func (c *Client) showTable() {
	c.document.Call("getElementById", "lobby_page").Set("className", "hidden")
	c.document.Call("getElementById", "table_page").Set("className", "")
}

func (c *Client) getTables() {
	resp, err := c.client.Get("tables")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&c.tables); err != nil {
		return
	}
}

func (c *Client) renderTables() {
	tablesList := c.document.Call("getElementById", "tables_list").Get("firstElementChild")
	tablesList.Set("innerHTML", "")

	for _, table := range c.tables {
		tableName := table.Name
		li := c.document.Call("createElement", "li")
		span := c.document.Call("createElement", "span")
		status := ""
		if table.IsPlaying {
			status = " - In Progress"
		}
		span.Set("textContent", fmt.Sprintf("%s (%d/%d players)%s",
			table.Name, table.PlayerCount, model.MaxTableSize, status))
		button := c.document.Call("createElement", "button")
		button.Set("textContent", "Join")
		button.Set("className", "join_table_button")
		button.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			c.joinTable(tableName)
			return nil
		}))
		li.Call("appendChild", span)
		li.Call("appendChild", button)
		tablesList.Call("appendChild", li)
	}
}

func (c *Client) createTable(this js.Value, args []js.Value) interface{} {
	tableName := c.document.Call("getElementById", "new_table_name_input").Get("value").String()
	if tableName == "" {
		return nil
	}

	go func() {
		username := c.document.Call("getElementById", "username_input").Get("value").String()
		if c.player != nil {
			username = c.player.Name
		}
		if err := c.post("tables", map[string]string{"name": tableName, "username": username}); err != nil {
			// Handle error, e.g., show a message to the user
			return
		}
		c.player = &model.Player{Name: username}
		c.joinTable(tableName)
	}()

	return nil
}

func (c *Client) post(endpoint string, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(endpoint, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	return nil
}

func (c *Client) joinTable(tableName string) {
	c.showTable()
	c.connect(tableName)
}

func (c *Client) connect(tableName string) {
	protocol := "ws"
	if js.Global().Get("location").Get("protocol").String() == "https:" {
		protocol = "wss"
	}
	host := js.Global().Get("location").Get("host").String()
	pathname := js.Global().Get("location").Get("pathname").String()

	c.conn = js.Global().Get("WebSocket").New(protocol + "://" + host + pathname + "ws?table=" + tableName)
	if c.conn.IsUndefined() {
		return
	}

	c.conn.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return nil
	}))
	c.conn.Set("onmessage", js.FuncOf(c.onMessage))
}

func (c *Client) onMessage(this js.Value, args []js.Value) interface{} {
	message := args[0].Get("data").String()
	var update gateway.ServerToPlayer
	if err := json.Unmarshal([]byte(message), &update); err != nil {
		return nil
	}

	switch update.Type {
	case gateway.PlayerUpdateT:
		switch update.PlayerUpdate.Type {
		case tableserver.FullUpdateT:
			c.renderFullTable(update.PlayerUpdate.Table)
			c.table = update.PlayerUpdate.Table
		case tableserver.NewHandUpdateT:
			c.renderHoleCards(update.PlayerUpdate.Hole)
		}
	case gateway.TableActionResponseT:
		// TODO check the action and if err is nil, update the UI accordingly.
	}

	return nil
}

func (c *Client) renderFullTable(table tableserver.SerializableTable) {
	for i, player := range table.Table.Players {
		seatIndex := i // Capture the loop variable
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
		playerName := seat.Call("querySelector", ".player_name")
		sitButton := seat.Call("querySelector", ".sit_down_button")

		if player != nil {
			playerName.Set("textContent", player.Name)
			sitButton.Get("classList").Call("add", "hidden")
		} else {
			playerName.Set("textContent", "")
			sitButton.Get("classList").Call("remove", "hidden")
			sitButton.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				c.sit(seatIndex)
				return nil
			}))
		}
	}

	if c.player != nil && c.player.Name == table.AdminName && !table.Playing {
		startButton := c.document.Call("getElementById", "start_game_button")
		startButton.Get("classList").Call("remove", "hidden")
		startButton.Set("onclick", js.FuncOf(c.start))
	}
}

func (c *Client) send(action interface{}) {
	if c.conn.IsUndefined() {
		return
	}
	json, err := json.Marshal(action)
	if err != nil {
		return
	}
	c.conn.Call("send", string(json))
}

func (c *Client) sit(seat int) {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Sit, TableName: "t", Seat: seat}})
}

func (c *Client) stand() {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Stand}})
}

func (c *Client) bet(amount int) {
	c.send(gateway.PlayerRequest{Type: gateway.RoundActionT, RoundAction: model.RoundAction{ActionType: model.Raise, Bet: amount}})
}

func (c *Client) fold() {
	c.send(gateway.PlayerRequest{Type: gateway.RoundActionT, RoundAction: model.RoundAction{ActionType: model.Fold}})
}

func (c *Client) start(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Start, TableName: c.table.Name}})
	return nil
}

func (c *Client) renderHoleCards(cards []poker.Card) {
	playerHand := c.document.Call("getElementById", "player_hand")
	playerHand.Set("innerHTML", "")
	for _, card := range cards {
		cardDiv := c.document.Call("createElement", "div")
		cardDiv.Set("className", "card")
		cardDiv.Set("textContent", card.String())
		playerHand.Call("appendChild", cardDiv)
	}
}
