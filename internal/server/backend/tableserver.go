package chessserver

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/chehsunliu/poker"
	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

const (
	// Stand is a player's attempt to stand up from the table.
	Stand = TableActionType(iota)
	// Sit is a player's attempt to sit at the table.
	Sit = TableActionType(iota)
	// Leave is a player's attempt to leave a table.
	Leave = TableActionType(iota)
	// Join is a player's attempt to join a table.
	Join = TableActionType(iota)
	// Create is a player's attempt to create a new table.
	Create = TableActionType(iota)
	// Start is a player's attempt to start a table's play.
	Start = TableActionType(iota)
	// Pause is a player's attempt to pause a table's play.
	Pause = TableActionType(iota)

	defaultTimeToBet        = time.Second * 30
	defaultTimeBetweenHands = time.Second * 5
)

type (
	// TableServerConfig defines the TableServer's behavior.
	TableServerConfig struct {
		maxConcurrentTables int
	}
	// TableGenerator generates TableConfigs.
	TableGenerator func() TableConfig
	// TableConfig defines the TableServer's opinion of how a given Table should be run.
	TableConfig struct {
		timeToBet        time.Duration
		timeBetweenHands time.Duration
		modelConfig      model.TableConfig
	}
	// Player is a struct representing a client, containing channels for communications
	// between the client and the the TableServer.
	Player struct {
		playerModel *model.Player
		// Channel for requests from the client directed to the server.
		requestChan chan model.RoundAction
		// Channel for responses to requests from the client.
		responseChan chan RoundActionResponse
		// Channel for responses to requests from the client.
		tableResponseChan chan TableActionResponse
		// Channel for updates to the Table's state.
		tableUpdateChan chan TableDetails
		// The player's current table if any
		table *Table
		// Only one client connected to the player at a time
		clientMutex sync.Mutex
	}
	// Table is an instance of an ongoing game the TableServer is hosting.
	Table struct {
		name        string
		tableConfig TableConfig
		adminName   string
		mutex       sync.Mutex
		table       *model.Table
		playing     bool
		paused      bool
		players     map[string]*Player
	}
	// TableServer is the server that orchestrates one or more ongoing Tables.
	TableServer struct {
		tableServerConfig TableServerConfig
		tables            map[string]*Table
		tableActions      chan TableAction
		mutex             *sync.Mutex
	}

	// TableActionType is the type of action a player can take on a table outside of an ongoing game.
	TableActionType int

	// TableAction is a player's request to the table outside the scope of a given round.
	TableAction struct {
		tableActionType TableActionType
		tableName       string
		seat            int
		tableConfig     TableConfig
		player          *Player
	}

	// TableActionResponse is a resposne to a client's TableAction.
	TableActionResponse struct {
		success bool
	}

	// RoundActionResponse is a resposne to a client's RoundAction.
	RoundActionResponse struct {
		success bool
	}

	// Opponent contains the details of a player needed by other players.
	Opponent struct {
		name    string
		stack   int
		bet     int
		playing bool
	}
	// TableDetails has all the details of the table a client needs (other players, bet amounts, etc).
	TableDetails struct {
		players     []*Opponent
		dealerIndex int
		pot         int
		playing     bool
		standers    []*Opponent
		myHole      []poker.Card
		board       []poker.Card
	}

	// TableUpdate represents an update to the client unrelated to a specific bet.
	TableUpdate struct {
		TableDetails TableDetails
	}
)

func DefaultTableGenerator() TableConfig {
	return TableConfig{
		timeToBet:        defaultTimeToBet,
		timeBetweenHands: defaultTimeBetweenHands,
		modelConfig:      model.DefaultConfig(),
	}
}

func (ts *TableServer) StartTableServer(quit chan bool) {
	go ts.Serve()
}

func (ts *TableServer) NewTable(name string, config model.TableConfig, creator string) error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	if len(ts.tables) >= ts.tableServerConfig.maxConcurrentTables {
		return errors.New("too many tables")
	} else if _, ok := ts.tables[name]; ok {
		return errors.New("duplicate name")
	}
	ts.tables[name] = &Table{
		name: name, table: model.NewTableWithConfig(config), adminName: creator,
		tableConfig: DefaultTableGenerator(),
	}
	return nil
}

// Serve handles table actions
func (ts *TableServer) Serve() {
	for tableAction := range ts.tableActions {
		switch tableAction.tableActionType {
		case Create:
			err := ts.NewTable(
				tableAction.tableName,
				tableAction.tableConfig.modelConfig,
				tableAction.player.playerModel.Name,
			)
			tableAction.player.tableResponseChan <- TableActionResponse{success: err == nil}
		case Stand:
			ts.mutex.Lock()
			err := tableAction.player.playerModel.StandUp()
			ts.mutex.Unlock()
			tableAction.player.tableResponseChan <- TableActionResponse{success: err == nil}
		case Sit:
			ts.mutex.Lock()
			err := tableAction.player.playerModel.GetTable().SitDown(
				tableAction.player.playerModel, tableAction.seat,
			)
			ts.mutex.Unlock()
			tableAction.player.tableResponseChan <- TableActionResponse{success: err == nil}
		case Leave:
			ts.mutex.Lock()
			// TODO if leaver is admin, update admin.
			err := tableAction.player.playerModel.Leave()
			ts.mutex.Unlock()
			tableAction.player.tableResponseChan <- TableActionResponse{success: err == nil}
		case Join:
			ts.mutex.Lock()
			var err error
			for _, t := range ts.tables {
				if t.name == tableAction.tableName {
					err = t.table.Join(tableAction.player.playerModel)
					break
				}
			}
			ts.mutex.Unlock()
			tableAction.player.tableResponseChan <- TableActionResponse{success: err == nil}
		case Start:
			go ts.ServeTable(ts.tables[tableAction.tableName])
		case Pause:
			ts.mutex.Lock()
			ts.tables[tableAction.tableName].mutex.Lock()
			ts.tables[tableAction.tableName].paused = true
			ts.tables[tableAction.tableName].mutex.Unlock()
			ts.mutex.Unlock()
		}
	}
}

func (t *Table) isPlaying() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.playing
}

func (t *Table) setPlaying(p bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.playing = p
}

func (t *Table) newHand() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.table.Hand = t.table.NewHand()
}

func (t *Table) startHand() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if err := t.table.Hand.StartHand(); err != nil {
		t.playing = false
		return err
	}
	return nil
}

func (t *Table) getDealer() *model.Player {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.Dealer()
}

func (t *Table) handDone() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.HandDone
}

func (t *Table) getBoard() []poker.Card {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.Board
}

func (t *Table) setHandDone(d bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.table.Hand.HandDone = d
}

func (t *Table) finishHand() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if err := t.table.Hand.FinishHand(); err != nil {
		t.playing = false
		return err
	}
	return nil
}

func (t *Table) getTimeToBet() time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.tableConfig.timeToBet
}

func (t *Table) getTimeBetweenHands() time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.tableConfig.timeBetweenHands
}

func (t *Table) handleStand() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for _, p := range t.table.Players {
		if p != nil && p.WantToStandUp {
			p.StandUp()
		}
	}
}

func (t *Table) incrementDealerIndex() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if err := t.table.IncrementDealerIndex(); err != nil {
		log.Println(err)
		t.playing = false
		return err
	}
	return nil
}

func (ts *TableServer) ServeTable(table *Table) error {
	// TODO handle table paused
	if table.isPlaying() {
		return errors.New("play: table already playing")
	}
	table.setPlaying(true)
	for {
		table.newHand()
		log.Println("Dealing next hand, dealer is", table.getDealer())
		if err := table.startHand(); err != nil {
			return err
		}
		table.ListenForPlayerActions()
		for !table.handDone() {
			table.table.Hand.Deal()
			table.ListenForPlayerActions()
			if len(table.getBoard()) == 5 {
				table.setHandDone(true)
			}
		}
		if err := table.finishHand(); err != nil {
			log.Println(err)
			return err
		}
		time.Sleep(table.getTimeBetweenHands())
		table.handleStand()
		if err := table.incrementDealerIndex(); err != nil {
			return err
		}
	}
}

func (t *Table) roundDone() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.Round.RoundDone
}

func (t *Table) setRoundDone(d bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.table.Hand.Round.RoundDone = d
}

func (t *Table) bettingDone() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.BettingDone
}

func (t *Table) currentBetter() *model.Player {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.Round.BetTurn.Value.(*model.Player)
}

func (t *Table) handlePlayerAction(player *model.Player, action model.RoundAction) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.table.Hand.PlayerAction(player, action)

}

// ListenForPlayerActions get each player's action for the round of bets
func (t *Table) ListenForPlayerActions() {
	for !t.roundDone() && !t.bettingDone() && !t.handDone() {
		success := false
		player := t.currentBetter()
		timeRemaining := t.getTimeToBet()
		for !success {
			ctx, cancel := context.WithTimeout(context.Background(), timeRemaining)
			defer cancel()
			n := time.Now()
			client := t.players[player.Name]
			err := t.handlePlayerAction(player, getPlayerAction(ctx, client))
			timeRemaining -= time.Since(n)
			if err == nil {
				success = true
			} else {
				log.Println(err)
			}
			client.responseChan <- RoundActionResponse{err == nil}
		}
		log.Println(player.Name, "made their bet")
	}
	log.Println("Round of betting is done")
	t.setRoundDone(true)
}

func getPlayerAction(ctx context.Context, player *Player) model.RoundAction {
	log.Println("Waiting for action from", player.playerModel.Name)
	action := model.RoundAction{ActionType: model.Fold}
	select {
	case action = <-player.requestChan:
	case <-ctx.Done():
		log.Println(player.playerModel.Name, "timed out, folding")
	}
	return action
}
