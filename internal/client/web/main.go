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
	"sync"
	"syscall/js"
	"time"

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
	allInButton       js.Value
	betTimer          js.Value
	betTimeRemaining  time.Duration
	betTimeElapsed    time.Duration
	betTimeLastUpdate time.Time
	mutex             sync.Mutex
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
		allInButton:       d.Call("getElementById", "all_in_button"),
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
	c.allInButton.Set("onclick", js.FuncOf(c.allIn))
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

func (c *Client) startBetTimer(seat int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !c.betTimer.IsUndefined() {
		js.Global().Call("clearInterval", c.betTimer)
	}
	timerBar := c.document.Call("getElementById", fmt.Sprintf("seat_%d", seat)).Call("querySelector", ".timer-bar")
	timerBar.Get("classList").Call("remove", "hidden")
	c.betTimer = js.Global().Call("setInterval", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		now := time.Now()
		c.betTimeElapsed += now.Sub(c.betTimeLastUpdate)
		c.betTimeLastUpdate = now
		timerBarWidth := 100 * float64(c.betTimeRemaining-c.betTimeElapsed) / float64(c.betTimeRemaining)
		timerBar.Get("style").Set("width", fmt.Sprintf("%f%%", timerBarWidth))
		if timerBarWidth <= 0 {
			js.Global().Call("clearInterval", c.betTimer)
		}
		return nil
	}), 1000)
}

func (c *Client) clearBetTimerLoop() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !c.betTimer.IsUndefined() {
		js.Global().Call("clearInterval", c.betTimer)
	}
}

func (c *Client) stopBetTimer(seat int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !c.betTimer.IsUndefined() {
		js.Global().Call("clearInterval", c.betTimer)
	}
	c.betTimeElapsed = 0
	timerBar := c.document.Call("getElementById", fmt.Sprintf("seat_%d", seat)).Call("querySelector", ".timer-bar")
	timerBar.Get("style").Set("width", "100.0%")
	timerBar.Get("classList").Call("add", "hidden")
}

func (c *Client) onMessage(this js.Value, args []js.Value) interface{} {
	message := args[0].Get("data").String()
	var update gateway.ServerToPlayer
	if err := json.Unmarshal([]byte(message), &update); err != nil {
		log.Println("Error unmarshaling", err)
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
			c.betTimeRemaining = update.PlayerUpdate.TimeRemaining
			c.betTimeElapsed = update.PlayerUpdate.TimeElapsed
			c.betTimeLastUpdate = time.Now()
			c.startBetTimer(update.PlayerUpdate.CurrentSeat)
			if update.PlayerUpdate.CurrentBetter == c.player.Name {
				c.document.Call("getElementById", "player_controls").Get("classList").Call("remove", "hidden")
			}
		case tableserver.TableUpdateT:
			c.handleTableUpdate(update.PlayerUpdate.TableAction)
		case tableserver.StateUpdateT:
			c.handleStateUpdate(update.PlayerUpdate.StateUpdate)
		case tableserver.RoundUpdateT:
			c.stopBetTimer(update.PlayerUpdate.CurrentSeat)
			action := update.PlayerUpdate.RoundAction
			switch action.ActionType {
			case model.Fold:
				c.logAction(fmt.Sprintf("%s folds.", update.PlayerUpdate.CurrentBetter))
				c.slideBetToPot(update.PlayerUpdate.CurrentSeat, update.PlayerUpdate.CurrentBets)
				if update.PlayerUpdate.CurrentBetter == c.player.Name {
					c.document.Call("getElementById", "player_hand").Set("innerHTML", "")
				}
			case model.Check:
				c.logAction(fmt.Sprintf("%s checks.", update.PlayerUpdate.CurrentBetter))
			case model.Call:
				c.logAction(fmt.Sprintf("%s calls.", update.PlayerUpdate.CurrentBetter))
			case model.Raise:
				c.logAction(fmt.Sprintf("%s raises to %d.", update.PlayerUpdate.CurrentBetter, action.Bet))
				c.document.Call("getElementById", "current_bet_amount").Set("value", action.Bet)
			case model.AllIn:
				c.logAction(fmt.Sprintf("%s is all in with %d.", update.PlayerUpdate.CurrentBetter, action.Bet))
				c.document.Call("getElementById", "current_bet_amount").Set("value", action.Bet)
			}
			if update.PlayerUpdate.RoundDone && !update.PlayerUpdate.HandDone {
				c.slideBetsToPot(update.PlayerUpdate)
			} else if !update.PlayerUpdate.HandDone && update.PlayerUpdate.RoundAction.ActionType != model.Fold {
				c.renderBets(update.PlayerUpdate.CurrentBets)
			}
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
				c.slideBetsToWinner(winner.Player.SeatIndex, make([]int, model.MaxTableSize))
				c.slidePotToWinner(winner.Player.SeatIndex)
			}
		}
	case gateway.TableActionResponseT:
		if update.TableActionResponse.Err != "" {
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
			c.pauseGameButton.Get("classList").Call("remove", "hidden")
			c.unpauseGameButton.Get("classList").Call("add", "hidden")
		case tableserver.Pause:
			c.pauseGameButton.Get("classList").Call("add", "hidden")
			c.unpauseGameButton.Get("classList").Call("remove", "hidden")
		case tableserver.Start:
			c.pauseGameButton.Get("classList").Call("remove", "hidden")
			c.startGameButton.Get("classList").Call("add", "hidden")
		}
	case gateway.RoundActionResponseT:
		if update.RoundActionResponse.Err != "" {
			c.logAction(fmt.Sprintf("Invalid bet: %s", update.RoundActionResponse.Err))
		} else {
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
		betDiv := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", i))
		betDiv.Set("innerHTML", "")
		betDiv.Get("style").Set("transform", "")
		betDiv.Get("style").Set("transition", "")
		if bet > 0 {
			c.renderBetChips(bet, betDiv)
		}
	}
}

func (c *Client) renderBetChips(bet int, betDiv js.Value) {
	originalBet := bet
	chipValues := []int{10000, 1000, 500, 100, 25, 5, 1}
	chipColors := []string{"brown", "yellow", "blue", "black", "green", "red", "white"}
	chipsAdded := 0
	maxChipHeight := 10
	stackDiv := c.document.Call("createElement", "div")
	for i, value := range chipValues {
		count := bet / value
		bet -= count * value
		for j := 0; j < count; j++ {
			chipColumn := chipsAdded / maxChipHeight
			chipRow := chipColumn % 2
			chip := c.document.Call("createElement", "img")
			chip.Set("src", fmt.Sprintf("assets/poker chips/%s.png", chipColors[i]))
			chip.Get("style").Set("width", "30px")
			chip.Get("style").Set("height", "30px")
			chip.Get("style").Set("position", "absolute")
			chip.Get("style").Set("bottom", fmt.Sprintf("%dpx", chipRow*7+(chipsAdded%maxChipHeight)*2))
			chip.Get("style").Set("left", fmt.Sprintf("%dpx", chipColumn*7))
			chip.Get("style").Set("z-index", 100+(chipRow*-maxChipHeight)+(chipsAdded/maxChipHeight))
			chipsAdded++
			stackDiv.Call("appendChild", chip)
			if chipsAdded%maxChipHeight == 0 {
				betDiv.Call("appendChild", stackDiv)
				stackDiv = c.document.Call("createElement", "div")
			}
		}
		if value > 25 {
			// Large value chips get their own stack
			if chipsAdded%maxChipHeight > 0 {
				chipsAdded += (maxChipHeight - (chipsAdded % maxChipHeight))
				betDiv.Call("appendChild", stackDiv)
				stackDiv = c.document.Call("createElement", "div")
			}
		}
	}
	if chipsAdded%maxChipHeight > 0 {
		betDiv.Call("appendChild", stackDiv)
	}
	if originalBet != 0 {
		betAmount := c.document.Call("createElement", "div")
		betAmount.Set("textContent", fmt.Sprintf("$%d", originalBet))
		betAmount.Get("style").Set("bottom", "-5px")
		betAmount.Get("style").Set("font-size", "0.7rem")
		betAmount.Get("style").Set("position", "absolute")
		betDiv.Call("appendChild", betAmount)
	} else {
		betDiv.Set("textContent", "")
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
	case tableserver.Pause:
		// TODO add UI showing pause
		c.clearBetTimerLoop()
	case tableserver.Unpause:
		// TODO remove UI showing pause
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
		c.renderPot(0)
	}
}

func (c *Client) removePlayer(s int, showButton bool) {
	seat := c.document.Call("getElementById", fmt.Sprintf("seat_%d", s))
	seat.Call("querySelector", ".player_name").Set("textContent", "")
	seat.Call("querySelector", ".player_funds").Set("textContent", "")
	playerBet := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", s))
	playerBet.Set("textContent", "")
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
		sitButton.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			c.sit(seatIndex)
			return nil
		}))

		if player != nil {
			playerName.Set("textContent", player.Name)
			sitButton.Get("classList").Call("add", "hidden")
			if c.player != nil && c.player.Name == player.Name {
				c.sitting = true
			}
			chipsDiv := seat.Call("querySelector", ".player_funds")
			if player.Funds > 0 {
				chipsDiv.Set("textContent", fmt.Sprintf("$%d", player.Funds))
			} else {
				chipsDiv.Set("textContent", "")
			}
		} else {
			playerName.Set("textContent", "")
			sitButton.Get("classList").Call("remove", "hidden")
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

func (c *Client) allIn(this js.Value, args []js.Value) interface{} {
	c.send(gateway.PlayerRequest{Type: gateway.RoundActionT, RoundAction: model.RoundAction{ActionType: model.AllIn}})
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
	potDiv.Set("textContent", "")
	if pot > 0 {
		potDiv.Set("textContent", fmt.Sprintf("$%d", pot))
		potDiv.Get("style").Set("font-size", "0.7rem")
	}
	c.renderPotChips(pot)
}

func (c *Client) renderPotChips(pot int) {
	chipValues := []int{10000, 1000, 500, 100, 25, 5, 1}
	chipColors := []string{"brown", "yellow", "blue", "black", "green", "red", "white"}
	chipsAdded := 0
	maxChipHeight := 10
	potChips := c.document.Call("getElementById", "pot_chips")
	potChips.Set("innerHTML", "")
	potChips.Get("style").Set("transition", "")
	potChips.Get("style").Set("transform", "")
	stackDiv := c.document.Call("createElement", "div")
	for i, value := range chipValues {
		count := pot / value
		pot -= count * value
		for j := 0; j < count; j++ {
			chipColumn := chipsAdded / maxChipHeight
			chipRow := chipColumn % 2
			chip := c.document.Call("createElement", "img")
			chip.Set("src", fmt.Sprintf("assets/poker chips/%s.png", chipColors[i]))
			chip.Get("style").Set("width", "30px")
			chip.Get("style").Set("height", "30px")
			chip.Get("style").Set("position", "absolute")
			chip.Get("style").Set("bottom", fmt.Sprintf("%dpx", chipRow*7+(chipsAdded%maxChipHeight)*2))
			chip.Get("style").Set("left", fmt.Sprintf("%dpx", chipColumn*7))
			chip.Get("style").Set("z-index", 100+(chipRow*-maxChipHeight)+(chipsAdded/maxChipHeight))
			chipsAdded++
			stackDiv.Call("appendChild", chip)
			if chipsAdded%maxChipHeight == 0 {
				potChips.Call("appendChild", stackDiv)
				stackDiv = c.document.Call("createElement", "div")
			}
		}
		if value > 25 {
			// Large value chips get their own stack
			if chipsAdded%maxChipHeight > 0 {
				chipsAdded += (maxChipHeight - (chipsAdded % maxChipHeight))
				potChips.Call("appendChild", stackDiv)
				stackDiv = c.document.Call("createElement", "div")
			}
		}
	}
	if chipsAdded%maxChipHeight > 0 {
		potChips.Call("appendChild", stackDiv)
	}
}

func (c *Client) slidePotToWinner(winnerSeat int) {
	potChips := c.document.Call("getElementById", "pot_chips")
	winnerBetEl := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", winnerSeat))
	winnerRect := winnerBetEl.Call("getBoundingClientRect")
	potRect := potChips.Call("getBoundingClientRect")
	potChips.Get("style").Set("transition", "all 1s ease-in-out")
	potChips.Get("style").Set("transform", fmt.Sprintf("translate(%dpx, %dpx)", winnerRect.Get("left").Int()-potRect.Get("left").Int(), winnerRect.Get("top").Int()-potRect.Get("top").Int()))
}

func (c *Client) slideBetToPot(seat int, currentBets []int) {
	potChips := c.document.Call("getElementById", "pot_chips")
	potRect := potChips.Call("getBoundingClientRect")
	betEl := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", seat))
	betRect := betEl.Call("getBoundingClientRect")
	betEl.Get("style").Set("transition", "all 1s ease-in-out")
	betEl.Get("style").Set("transform", fmt.Sprintf("translate(%dpx, %dpx)", potRect.Get("left").Int()-betRect.Get("left").Int(), potRect.Get("top").Int()-betRect.Get("top").Int()))
	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.renderBets(currentBets)
		return nil
	}), 1000)
}

func (c *Client) slideBetsToPot(update *tableserver.PlayerUpdate) {
	c.renderBets(update.CurrentBets)
	potChips := c.document.Call("getElementById", "pot_chips")
	potRect := potChips.Call("getBoundingClientRect")
	for i := 0; i < model.MaxTableSize; i++ {
		betEl := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", i))
		betRect := betEl.Call("getBoundingClientRect")
		betEl.Get("style").Set("transition", "all 1s ease-in-out")
		betEl.Get("style").Set("transform", fmt.Sprintf("translate(%dpx, %dpx)", potRect.Get("left").Int()-betRect.Get("left").Int(), potRect.Get("top").Int()-betRect.Get("top").Int()))
	}
	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.renderBets(make([]int, model.MaxTableSize))
		newPot := update.Pot
		for _, bet := range update.CurrentBets {
			newPot += bet
		}
		c.renderPot(newPot)
		return nil
	}), 1000)
}

func (c *Client) slideBetsToWinner(winnerSeat int, currentBets []int) {
	winnerBetEl := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", winnerSeat))
	winnerRect := winnerBetEl.Call("getBoundingClientRect")
	for i := 0; i < model.MaxTableSize; i++ {
		betEl := c.document.Call("getElementById", fmt.Sprintf("player_bet_%d", i))
		betRect := betEl.Call("getBoundingClientRect")
		betEl.Get("style").Set("transition", "all 1s ease-in-out")
		betEl.Get("style").Set("transform", fmt.Sprintf("translate(%dpx, %dpx)", winnerRect.Get("left").Int()-betRect.Get("left").Int(), winnerRect.Get("top").Int()-betRect.Get("top").Int()))
	}
	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.renderBets(currentBets)
		return nil
	}), 1000)
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
			fundsDiv.Set("textContent", fmt.Sprintf("$%d", f))
		} else {
			fundsDiv.Set("textContent", "")
		}
	}
}
