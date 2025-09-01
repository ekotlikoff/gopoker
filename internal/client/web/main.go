//go:build wasm && js && webclient

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strconv"
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
	sitting  bool
	playing  bool
	bigBlind int
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
		c.getSession()
		c.showLobby()
		c.getTables()
		c.renderTables()
	}()
	js.Global().Get("document").Call("getElementById", "login_button").Set("onclick", js.FuncOf(c.login))
	js.Global().Get("document").Call("getElementById", "create_table_button").Set("onclick", js.FuncOf(c.createTable))
}

func (c *Client) login(this js.Value, args []js.Value) interface{} {
	username := c.document.Call("getElementById", "username_input").Get("value").String()
	if username == "" {
		return nil
	}

	go func() {
		if err := c.post("login", map[string]string{"username": username}); err != nil {
			// Handle error, e.g., show a message to the user
			return
		}
		c.player = model.NewPlayer(username)
		c.document.Call("getElementById", "username_input").Set("value", username)
		c.document.Call("getElementById", "username_input").Set("disabled", true)
		c.document.Call("getElementById", "login_button").Get("classList").Call("add", "hidden")
	}()

	return nil

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
	c.document.Call("getElementById", "username_input").Set("value", session.Credentials.Username)
	c.document.Call("getElementById", "username_input").Set("disabled", true)
	c.document.Call("getElementById", "login_button").Get("classList").Call("add", "hidden")
	return true
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
		if err := c.post("tables", map[string]string{"name": tableName}); err != nil {
			// Handle error, e.g., show a message to the user
			return
		}
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
			c.bigBlind = update.PlayerUpdate.BigBlind
			c.renderBets(update.PlayerUpdate.CurrentBets)
			c.document.Call("getElementById", "current_bet_amount").Set("value", c.bigBlind)
			c.renderPot(update.PlayerUpdate.Pot)
			c.renderFunds(update.PlayerUpdate.CurrentFunds)
		case tableserver.BetUpdateT:
			c.document.Call("getElementById", "player_controls").Get("classList").Call("remove", "hidden")
		case tableserver.TableUpdateT:
			c.handleTableUpdate(update.PlayerUpdate.TableAction)
		case tableserver.RoundUpdateT:
			c.renderBets(update.PlayerUpdate.CurrentBets)
			c.renderPot(update.PlayerUpdate.Pot)
			c.renderFunds(update.PlayerUpdate.CurrentFunds)
		}
	case gateway.TableActionResponseT:
		if update.TableActionResponse.Err != nil {
			// TODO: display error to user
			return nil
		}
		switch update.TableActionResponse.TableAction.TableActionType {
		case tableserver.Sit:
			if update.TableActionResponse.Err == nil {
				c.sitting = true
				// Show the stand button
				c.document.Call("getElementById", "stand_button").Get("classList").Call("remove", "hidden")
				// Hide the sit buttons
				for i := 0; i < model.MaxTableSize; i++ {
					seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
					sitButton := seat.Call("querySelector", ".sit_down_button")
					sitButton.Get("classList").Call("add", "hidden")
				}
			}
		case tableserver.Stand:
			if update.TableActionResponse.Err == nil {
				c.sitting = false
				// Hide the stand button
				c.document.Call("getElementById", "stand_button").Get("classList").Call("add", "hidden")
				// Show the sit buttons
				for i := 0; i < model.MaxTableSize; i++ {
					seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
					player := seat.Call("querySelector", ".player_name").Get("textContent").String()
					if player == "" {
						seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
						sitButton := seat.Call("querySelector", ".sit_down_button")
						sitButton.Get("classList").Call("remove", "hidden")
					}
				}
			}
		case tableserver.Join:
			// TODO
		case tableserver.Leave:
			// TODO
		case tableserver.Create:
			// TODO
		case tableserver.Start:
			// TODO
		}
	case gateway.RoundActionResponseT:
		if update.RoundActionResponse.Err == nil {
			c.document.Call("getElementById", "player_controls").Get("classList").Call("add", "hidden")
		}
	}

	return nil
}

func (c *Client) renderBets(currentBets []int) {
	for i, bet := range currentBets {
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
		betDiv := seat.Call("querySelector", ".player_bet")
		if bet != 0 {
			betDiv.Set("textContent", fmt.Sprintf("Bet: %d", bet))
		} else {
			betDiv.Set("textContent", "")
		}
	}
}

func (c *Client) handleTableUpdate(action tableserver.TableAction) {
	switch action.TableActionType {
	case tableserver.Sit:
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", action.Seat))
		playerName := seat.Call("querySelector", ".player_name")
		playerName.Set("textContent", action.PlayerName)
		seat.Call("querySelector", ".sit_down_button").Get("classList").Call("add", "hidden")
	case tableserver.Stand:
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", action.Seat))
		playerName := seat.Call("querySelector", ".player_name")
		playerName.Set("textContent", "")
		if !c.sitting {
			seat.Call("querySelector", ".sit_down_button").Get("classList").Call("remove", "hidden")
		}
	}
}

func (c *Client) renderFullTable(table tableserver.SerializableTable) {
	c.sitting = false
	for i, player := range table.Table.Players {
		seatIndex := i // Capture the loop variable
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
		playerName := seat.Call("querySelector", ".player_name")
		sitButton := seat.Call("querySelector", ".sit_down_button")

		if player != nil {
			playerName.Set("textContent", player.Name)
			sitButton.Get("classList").Call("add", "hidden")
			if c.player != nil && c.player.Name == player.Name {
				c.sitting = true
			}
			chipsDiv := seat.Call("querySelector", ".player_chips")
			if player.Funds > 0 {
				chipsDiv.Set("textContent", fmt.Sprintf("Chips: %d", player.Funds))
			} else {
				chipsDiv.Set("textContent", "")
			}
		} else {
			playerName.Set("textContent", "")
			sitButton.Get("classList").Call("remove", "hidden")
			sitButton.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				c.sit(seatIndex)
				return nil
			}))
		}
	}

	if c.sitting {
		for i := 0; i < model.MaxTableSize; i++ {
			seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
			sitButton := seat.Call("querySelector", ".sit_down_button")
			sitButton.Get("classList").Call("add", "hidden")
		}
	}

	if c.player.Name == table.AdminName && !table.Playing {
		startButton := c.document.Call("getElementById", "start_game_button")
		startButton.Get("classList").Call("remove", "hidden")
		startButton.Set("onclick", js.FuncOf(c.start))
	}

	standButton := c.document.Call("getElementById", "stand_button")
	standButton.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.stand()
		return nil
	}))
	if c.sitting {
		standButton.Get("classList").Call("remove", "hidden")
	}

	betButton := c.document.Call("getElementById", "bet_button")
	betButton.Set("onclick", js.FuncOf(c.bet))

	foldButton := c.document.Call("getElementById", "fold_button")
	foldButton.Set("onclick", js.FuncOf(c.fold))
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
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Sit, TableName: c.table.Name, Seat: seat}})
}

func (c *Client) stand() {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Stand, TableName: c.table.Name}})
}

func (c *Client) bet(this js.Value, args []js.Value) interface{} {
	betAmount, _ := strconv.Atoi(c.document.Call("getElementById", "current_bet_amount").Get("value").String())
	c.send(gateway.PlayerRequest{Type: gateway.RoundActionT, RoundAction: model.RoundAction{ActionType: model.Raise, Bet: betAmount}})
	return nil
}

func (c *Client) fold(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.RoundActionT, RoundAction: model.RoundAction{ActionType: model.Fold}})
	return nil
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

func (c *Client) renderPot(pot int) {
	potDiv := c.document.Call("getElementById", "pot")
	potDiv.Set("textContent", fmt.Sprintf("Pot: %d", pot))
}

func (c *Client) renderFunds(funds []int) {
	for i, f := range funds {
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
		fundsDiv := seat.Call("querySelector", ".player_funds")
		if f > 0 {
			fundsDiv.Set("textContent", fmt.Sprintf("Chips: %d", f))
		} else {
			fundsDiv.Set("textContent", "")
		}
	}
}
