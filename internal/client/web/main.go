//go:build wasm && js && webclient

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/chehsunliu/poker"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
	tableserver "github.com/ekotlikoff/gopoker/internal/server/backend"
	gateway "github.com/ekotlikoff/gopoker/internal/server/frontend"
)

// Looks like our poker library doesn't define consts for these.
const (
	spade   int32 = 1
	heart   int32 = 2
	diamond int32 = 4
	club    int32 = 8
)

var (
	prettySuits = map[int32]string{
		1: "\u2660", // spades
		2: "\u2764", // hearts
		4: "\u2666", // diamonds
		8: "\u2663", // clubs
	}
	strRanks = "23456789TJQKA"
)

type Client struct {
	document          js.Value
	client            *http.Client
	conn              js.Value // WebSocket connection
	player            *model.Player
	table             tableserver.SerializableTable
	tables            []model.TableSummary
	sitting           bool
	bigBlind          int
	actionLog         []string
	loginButton       js.Value
	createTableButton js.Value
	leaveTableButton  js.Value
	standButton       js.Value
	startGameButton   js.Value
	pauseGameButton   js.Value
	unpauseGameButton js.Value
	betButton         js.Value
	foldButton        js.Value
}

func main() {
	done := make(chan struct{})
	c := makeClient()
	c.initLobby()
	<-done
}

func makeClient() *Client {
	jar, _ := cookiejar.New(&cookiejar.Options{})
	d := js.Global().Get("document")
	return &Client{
		document:          d,
		client:            &http.Client{Jar: jar},
		loginButton:       d.Call("getElementById", "login_button"),
		createTableButton: d.Call("getElementById", "create_table_button"),
		leaveTableButton:  d.Call("getElementById", "leave_table_button"),
		standButton:       d.Call("getElementById", "stand_button"),
		startGameButton:   d.Call("getElementById", "start_game_button"),
		pauseGameButton:   d.Call("getElementById", "pause_game_button"),
		unpauseGameButton: d.Call("getElementById", "unpause_game_button"),
		betButton:         d.Call("getElementById", "bet_button"),
		foldButton:        d.Call("getElementById", "fold_button"),
	}
}

func (c *Client) initLobby() {
	go func() {
		c.getSession()
		c.showLobby()
		c.getTables()
		c.renderTables()
	}()
	c.loginButton.Set("onclick", js.FuncOf(c.login))
	c.createTableButton.Set("onclick", js.FuncOf(c.createTable))
	c.leaveTableButton.Set("onclick", js.FuncOf(c.leaveTable))
	c.standButton.Set("onclick", js.FuncOf(c.stand))
	c.startGameButton.Set("onclick", js.FuncOf(c.start))
	c.pauseGameButton.Set("onclick", js.FuncOf(c.pause))
	c.unpauseGameButton.Set("onclick", js.FuncOf(c.unpause))
	c.betButton.Set("onclick", js.FuncOf(c.bet))
	c.foldButton.Set("onclick", js.FuncOf(c.fold))
}

func (c *Client) login(this js.Value, args []js.Value) interface{} {
	username := c.document.Call("getElementById", "username_input").Get("value").String()
	if username == "" {
		return nil
	}

	go func() {
		if err := c.post("login", map[string]string{"username": username}); err != nil {
			// Handle error, e.g., show a message to the user
			log.Println("login failed")
			return
		}
		c.player = model.NewPlayer(username)
		c.document.Call("getElementById", "username_input").Set("value", username)
		c.document.Call("getElementById", "username_input").Set("disabled", true)
		c.loginButton.Get("classList").Call("add", "hidden")
	}()

	return nil

}

func (c *Client) getSession() bool {
	resp, err := c.client.Get("session")
	if err != nil || resp.StatusCode < 200 || resp.StatusCode > 299 {
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
	c.loginButton.Get("classList").Call("add", "hidden")
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
			log.Println("Failed to create a table")
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
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf(resp.Status)
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
		log.Println("Websocket connection is undefined.")
		c.showLobby()
		return
	}

	c.conn.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return nil
	}))
	c.conn.Set("onmessage", js.FuncOf(c.onMessage))
	c.conn.Set("onclose", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		log.Println("Websocket connection closed, returning to lobby")
		c.showLobby()
		return nil
	}))
}

func (c *Client) logAction(message string) {
	c.actionLog = append(c.actionLog, message)
	// Garbage collect old messages.
	if len(c.actionLog) > 50 {
		c.actionLog = c.actionLog[len(c.actionLog)-50 : len(c.actionLog)]
	}
	actionLog := c.document.Call("getElementById", "action_log")
	actionLog.Set("innerHTML", strings.Join(c.actionLog, "<br>"))
	actionLog.Set("scrollTop", actionLog.Get("scrollHeight"))
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
			c.logAction("New hand dealt.")
			communityCardsDiv := c.document.Call("getElementById", "community_cards")
			communityCardsDiv.Set("innerHTML", "")
			c.renderDealer(update.PlayerUpdate.Dealer)
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
		case tableserver.StateUpdateT:
			c.handleStateUpdate(update.PlayerUpdate.StateUpdate)
		case tableserver.RoundUpdateT:
			action := update.PlayerUpdate.RoundAction
			switch action.ActionType {
			case model.Fold:
				c.logAction(fmt.Sprintf("%s folds.", update.PlayerUpdate.CurrentBetter))
			case model.Check:
				c.logAction(fmt.Sprintf("%s checks.", update.PlayerUpdate.CurrentBetter))
			case model.Call:
				c.logAction(fmt.Sprintf("%s calls.", update.PlayerUpdate.CurrentBetter))
			case model.Raise:
				c.logAction(fmt.Sprintf("%s raises to %d.", update.PlayerUpdate.CurrentBetter, action.Bet))
				c.document.Call("getElementById", "current_bet_amount").Set("value", action.Bet)
			}
			if update.PlayerUpdate.RoundAction.ActionType == model.Fold && update.PlayerUpdate.CurrentBetter == c.player.Name {
				c.document.Call("getElementById", "player_hand").Set("innerHTML", "")
			}
			c.renderBets(update.PlayerUpdate.CurrentBets)
			c.renderPot(update.PlayerUpdate.Pot)
			c.renderFunds(update.PlayerUpdate.CurrentFunds)
		case tableserver.DealUpdateT:
			c.renderCommunityCards(update.PlayerUpdate.Board)
		case tableserver.HandOverUpdateT:
			for _, winner := range update.PlayerUpdate.Winners {
				if winner.Player.Hole != nil {
					c.logAction(fmt.Sprintf("%s wins %d chips with %s.", winner.Player.Name, winner.Winnings, poker.RankString(winner.Player.HandRank)))
				} else {
					// Player did not show their cards.
					c.logAction(fmt.Sprintf("%s wins %d chips.", winner.Player.Name, winner.Winnings))
				}
			}
		}
	case gateway.TableActionResponseT:
		if update.TableActionResponse.Err != nil {
			log.Println("error", update.TableActionResponse.TableAction.TableActionType)
			// TODO: display error to user
			return nil
		}
		switch update.TableActionResponse.TableAction.TableActionType {
		case tableserver.Sit:
			c.sitting = true
			c.player.SeatIndex = update.TableActionResponse.TableAction.Seat
			seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", c.player.SeatIndex))
			playerName := seat.Call("querySelector", ".player_name")
			playerName.Get("classList").Call("add", "current-player")
			c.standButton.Get("classList").Call("remove", "hidden")
			c.leaveTableButton.Get("classList").Call("add", "hidden")
			for i := 0; i < model.MaxTableSize; i++ {
				seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
				sitButton := seat.Call("querySelector", ".sit_down_button")
				sitButton.Get("classList").Call("add", "hidden")
			}
		case tableserver.Stand:
			// TODO display a pending stand... in the UI, and display a cancel stand button.
		case tableserver.Join:
			// TODO
		case tableserver.Leave:
			c.showLobby()
		case tableserver.Create:
			// TODO
		case tableserver.Unpause:
			// TODO remove UI showing pause
			if c.player.Name == c.table.AdminName {
				c.pauseGameButton.Get("classList").Call("remove", "hidden")
				c.unpauseGameButton.Get("classList").Call("add", "hidden")
			}
		case tableserver.Pause:
			// TODO add UI showing pause
			if c.player.Name == c.table.AdminName {
				c.pauseGameButton.Get("classList").Call("add", "hidden")
				c.unpauseGameButton.Get("classList").Call("remove", "hidden")
			}
		case tableserver.Start:
			if c.player.Name == c.table.AdminName {
				c.pauseGameButton.Get("classList").Call("remove", "hidden")
				c.startGameButton.Get("classList").Call("add", "hidden")
			}
		}
	case gateway.RoundActionResponseT:
		if update.RoundActionResponse.Err == nil {
			c.document.Call("getElementById", "player_controls").Get("classList").Call("add", "hidden")
		}
	}

	return nil
}

func (c *Client) renderDealer(dealerName string) {
	for i := 0; i < model.MaxTableSize; i++ {
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
		dealerChip := seat.Call("querySelector", ".dealer-chip")
		dealerChip.Get("classList").Call("add", "hidden")
		playerName := seat.Call("querySelector", ".player_name").Get("textContent").String()
		if playerName == dealerName {
			dealerChip.Get("classList").Call("remove", "hidden")
		}
	}
}

func (c *Client) removeDealerChip() {
	for i := 0; i < model.MaxTableSize; i++ {
		seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
		dealerChip := seat.Call("querySelector", ".dealer-chip")
		dealerChip.Get("classList").Call("add", "hidden")
	}
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
		// TODO hide the standing... UI
		if action.PlayerName == c.player.Name {
			seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", c.player.SeatIndex))
			playerName := seat.Call("querySelector", ".player_name")
			playerName.Get("classList").Call("remove", "current-player")
			c.leaveTableButton.Get("classList").Call("remove", "hidden")
			c.sitting = false
			c.standButton.Get("classList").Call("add", "hidden")
			c.leaveTableButton.Get("classList").Call("remove", "hidden")
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
		c.removePlayer(action.Seat, !c.sitting)
	}
}

func (c *Client) handleStateUpdate(stateUpdate tableserver.StateUpdate) {
	if stateUpdate.PlayStopped {
		communityCardsDiv := c.document.Call("getElementById", "community_cards")
		communityCardsDiv.Set("innerHTML", "")
		playerHand := c.document.Call("getElementById", "player_hand")
		playerHand.Set("innerHTML", "")
		if c.player.Name == c.table.AdminName {
			c.startGameButton.Get("classList").Call("remove", "hidden")
			c.pauseGameButton.Get("classList").Call("add", "hidden")
		}
		c.removeDealerChip()
	}
}

func (c *Client) removePlayer(s int, showButton bool) {
	seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", s))
	seat.Call("querySelector", ".player_name").Set("textContent", "")
	seat.Call("querySelector", ".player_bet").Set("textContent", "")
	seat.Call("querySelector", ".player_funds").Set("textContent", "")
	sitButton := seat.Call("querySelector", ".sit_down_button")
	if showButton {
		sitButton.Get("classList").Call("remove", "hidden")
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
			chipsDiv := seat.Call("querySelector", ".player_funds")
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

	if table.Table.Hand != nil {
		c.renderCommunityCards(table.Table.Hand.Board)
	}

	if c.sitting {
		c.standButton.Get("classList").Call("remove", "hidden")
		c.leaveTableButton.Get("classList").Call("add", "hidden")
		for i := 0; i < model.MaxTableSize; i++ {
			seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", i))
			sitButton := seat.Call("querySelector", ".sit_down_button")
			sitButton.Get("classList").Call("add", "hidden")
		}
	}

	if c.player.Name == table.AdminName {
		if table.Paused {
			c.unpauseGameButton.Get("classList").Call("remove", "hidden")
		} else if table.Playing {
			c.pauseGameButton.Get("classList").Call("remove", "hidden")
		} else {
			c.startGameButton.Get("classList").Call("remove", "hidden")
		}
	} else {
		c.startGameButton.Get("classList").Call("add", "hidden")
		c.pauseGameButton.Get("classList").Call("add", "hidden")
		c.unpauseGameButton.Get("classList").Call("add", "hidden")
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
	// Defer a function to recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v", r)
			log.Printf("Returning to lobby.")
			c.showLobby()
		}
	}()
	c.conn.Call("send", string(json))
}

func (c *Client) sit(seat int) {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Sit, TableName: c.table.Name, Seat: seat}})
}

func (c *Client) stand(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Stand, TableName: c.table.Name}})
	return nil
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

func (c *Client) pause(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Pause, TableName: c.table.Name}})
	return nil
}

func (c *Client) unpause(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Unpause, TableName: c.table.Name}})
	return nil
}

func (c *Client) leaveTable(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.TableActionT, TableAction: tableserver.TableAction{TableActionType: tableserver.Leave}})
	return nil
}

func (c *Client) renderHoleCards(cards []poker.Card) {
	playerHand := c.document.Call("getElementById", "player_hand")
	playerHand.Set("innerHTML", "")
	for _, card := range cards {
		cardDiv := c.document.Call("createElement", "div")
		cardDiv.Set("className", "card")
		if card.Suit() == spade || card.Suit() == club {
			cardDiv.Get("classList").Call("add", "black")
		} else {
			cardDiv.Get("classList").Call("add", "red")
		}
		rankDiv := c.document.Call("createElement", "div")
		rankDiv.Set("textContent", string(strRanks[card.Rank()]))
		suitDiv := c.document.Call("createElement", "div")
		suitDiv.Set("className", "suit")
		suitDiv.Set("textContent", prettySuits[card.Suit()])
		cardDiv.Call("appendChild", rankDiv)
		cardDiv.Call("appendChild", suitDiv)
		playerHand.Call("appendChild", cardDiv)
	}
}

func (c *Client) renderPot(pot int) {
	potDiv := c.document.Call("getElementById", "pot")
	potDiv.Set("textContent", fmt.Sprintf("Pot: %d", pot))
}

func (c *Client) renderCommunityCards(cards []poker.Card) {
	communityCardsDiv := c.document.Call("getElementById", "community_cards")
	communityCardsDiv.Set("innerHTML", "")
	for _, card := range cards {
		cardDiv := c.document.Call("createElement", "div")
		cardDiv.Set("className", "card")
		if card.Suit() == spade || card.Suit() == club {
			cardDiv.Get("classList").Call("add", "black")
		} else {
			cardDiv.Get("classList").Call("add", "red")
		}
		rankDiv := c.document.Call("createElement", "div")
		rankDiv.Set("textContent", string(strRanks[card.Rank()]))
		suitDiv := c.document.Call("createElement", "div")
		suitDiv.Set("className", "suit")
		suitDiv.Set("textContent", prettySuits[card.Suit()])
		cardDiv.Call("appendChild", rankDiv)
		cardDiv.Call("appendChild", suitDiv)
		communityCardsDiv.Call("appendChild", cardDiv)
	}
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
