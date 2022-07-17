package model

import (
	"container/ring"
	"errors"
	"fmt"
	"log"
	"time"

	// https://jonathanhsiao.com/blog/evaluating-poker-hands-with-bit-math
	// Poker hands are represented by bit fields, one which represents
	// the face values of the hand, and another which represents the count of
	// each card.  With fancy bit math these representations can very quickly
	// give the rank of a hand.
	"github.com/chehsunliu/poker"
)

const (
	// DefaultMinBet default minimum bet controlling big blinds
	DefaultMinBet = 200
	// MinPlayersToPlay below which the hand cannot start
	MinPlayersToPlay = 2
	// MaxTableSize once reached no more players can sit
	MaxTableSize = 10
	// MaxStandersSize once reached no more players can stand TODO what happens when standers is full and someone stands up?
	MaxStandersSize = 10
	// AllIn takes the player all in
	AllIn = ActionType(iota)
	// Raise the current bet
	Raise = ActionType(iota)
	// Call the current bet
	Call = ActionType(iota)
	// Fold your hand
	Fold = ActionType(iota)
)

type (
	// Player a player's state at a Table
	Player struct {
		Name          string
		Standing      bool
		WantToStandUp bool
		Playing       bool
		AllIn         bool
		Hole          []poker.Card
		Funds         int
		BetAmount     int
		HandRank      int32
		table         *Table
	}

	// PlayerBet a bet that is made in a round
	PlayerBet struct {
		Player *Player
		Bet    int
	}

	// Table the group of players playing hands or standing and watching
	Table struct {
		TableConfig TableConfig
		Players     [MaxTableSize]*Player
		DealerIndex int
		playing     bool
		Standers    [MaxStandersSize]*Player
		Hand        *Hand
	}

	// TableConfig define nuances of the game played at a Table
	TableConfig struct {
		minBet              int
		timeToBet           time.Duration
		secondsBetweenHands time.Duration
	}

	// ActionType an action a player can take during their turn in a round
	ActionType int

	// RoundAction how a player (another goroutine) can interact with the table during their turn in a round
	RoundAction struct {
		actionType ActionType
		bet        int
	}
)

// NewTable create a new table
func NewTable() *Table {
	table := NewTableWithConfig(
		TableConfig{
			minBet: DefaultMinBet, timeToBet: time.Second * 30,
			secondsBetweenHands: time.Second * 5,
		},
	)
	return table
}

// NewTableWithConfig create a new table with custom config
func NewTableWithConfig(tableConfig TableConfig) *Table {
	table := Table{TableConfig: tableConfig}
	return &table
}

// NewPlayer create a new player
func NewPlayer(name string) *Player {
	return NewPlayerWithFunds(name, 0)
}

// NewPlayerWithFunds create a new player with funds
func NewPlayerWithFunds(name string, funds int) *Player {
	player := Player{
		Name: name, Funds: funds,
	}
	return &player
}

// Returns ring starting at the dealer
func (table *Table) playersForHand() (*ring.Ring, Pot) {
	mainPot := SubPot{make(map[*Player]struct{}), 0}
	index := (table.DealerIndex + 1) % len(table.Players)
	var playersPlaying []*Player
	for i := 0; i < len(table.Players); i++ {
		player := table.Players[index]
		if player != nil {
			if player.Funds <= 0 {
				player.Standing = true
				table.Players[index] = nil
			} else {
				playersPlaying = append(playersPlaying, player)
				mainPot.Players[player] = struct{}{}
			}
		}
		index = (index + 1) % len(table.Players)
	}
	out := ring.New(len(playersPlaying))
	for _, p := range playersPlaying {
		out.Value = p
		out = out.Next()
	}
	return out.Prev(), Pot{MainPot: mainPot, SidePots: []SubPot{}}
}

func (table *Table) incrementDealerIndex() error {
	log.Printf("dealer index: %d\n", table.DealerIndex)
	for i := 1; i < len(table.Players); i++ {
		dealerIndex := (i + table.DealerIndex) % len(table.Players)
		log.Println("index", dealerIndex)
		player := table.Players[dealerIndex]
		if player != nil && player.Playing && i != table.DealerIndex {
			log.Printf("found player: %s, index: %d", player.Name, dealerIndex)
			table.DealerIndex = dealerIndex
			return nil
		}
	}
	return errors.New("incrementdealerindex: could not find next dealer")
}

// SitDown seat a player at the table
func (table *Table) SitDown(player *Player, seat int) error {
	if player.Funds < table.TableConfig.minBet {
		return errors.New("Player has insufficient funds to sit")
	} else if seat >= MaxTableSize {
		return errors.New("Seat, " + fmt.Sprint(seat) +
			" is greater than max table size, " + fmt.Sprint(MaxTableSize))
	} else if table.Players[seat] == nil {
		table.Players[seat] = player
		return nil
	} else {
		return errors.New("Seat is occupied, " + fmt.Sprint(seat))
	}
}

// StandUp a player at the next chance
func (player *Player) StandUp() {
	if player.Playing {
		player.WantToStandUp = true
	} else {
		player.GetTable().standUp(player)
	}
}

func (table *Table) standUp(player *Player) error {
	for i, p := range table.Players {
		if p == player {
			table.Players[i].Playing = false
			table.Players[i].Standing = true
			table.Players[i].WantToStandUp = false
			table.Players[i] = nil
			return nil
		}
	}
	return errors.New("Player is not sitting at this table")
}

// String player's string
func (player Player) String() string {
	cards := ""
	betAmount := ""
	if player.Playing {
		if len(player.Hole) > 0 {
			cards = ", Cards: "
		}
		for _, c := range player.Hole {
			cards += fmt.Sprint(c) + " "
		}
		betAmount = fmt.Sprintf(", BetAmount: %d", player.BetAmount)
	} else {
		betAmount = ", not playing"
	}
	return fmt.Sprintf("%s, funds: %v%v %s", player.Name, player.Funds, betAmount, cards)
}

// String table's string
func (table *Table) String() string {
	out := fmt.Sprintf("playing: %v, bettingDone: %v, handDone: %v\n", table.playing, table.Hand.BettingDone, table.Hand.HandDone)
	if len(table.Hand.Board) > 0 {
		out += "Board="
		for _, c := range table.Hand.Board {
			out += fmt.Sprint(c) + " "
		}
		out += "\n"
	}
	for _, pot := range append(table.Hand.Pot.SidePots, table.Hand.Pot.MainPot) {
		if pot.Pot != 0 {
			out += "Pot=" + fmt.Sprint(pot.Pot) +
				", player_count=" + fmt.Sprint(len(pot.Players)) + "\n"
		}
	}
	for i, p := range table.Players {
		seat := "Seat: " + fmt.Sprint(i) + ", " + fmt.Sprint(p)
		out += seat
		if p == RingToPlayer(table.Hand.Round.BetTurn) {
			out += " (B) "
		}
		if p == table.Hand.Dealer() {
			out += " (D) "
		}
		out += "\n"
	}
	return out
}

// GetTable get the player's table
func (player *Player) GetTable() *Table {
	return player.table
}

// // Play rounds at the table
// func (table *Table) Play() error {
// 	if table.playing {
// 		return errors.New("play: table already playing")
// 	}
// 	table.playing = true
// 	for {
// 		table.Hand = table.NewHand()
// 		log.Println("Dealing next hand, dealer is", RingToPlayer(table.Hand.Players).Name)
// 		if err := table.Hand.StartHand(); err != nil {
// 			table.playing = false
// 			return err
// 		}
// 		table.Hand.ListenForPlayerActions()
// 		for !table.Hand.HandDone {
// 			table.Hand.Deal()
// 			table.Hand.ListenForPlayerActions()
// 			if len(table.Hand.Board) == 5 {
// 				table.Hand.HandDone = true
// 			}
// 		}
// 		if err := table.Hand.FinishHand(); err != nil {
// 			log.Println(err)
// 			table.playing = false
// 			return err
// 		}
// 		time.Sleep(time.Second * table.TableConfig.secondsBetweenHands)
// 		for _, p := range table.Players {
// 			if p != nil && p.WantToStandUp {
// 				table.standUp(p)
// 			}
// 		}
// 		if err := table.incrementDealerIndex(); err != nil {
// 			log.Println(err)
// 			table.playing = false
// 			return err
// 		}
// 	}
// }
