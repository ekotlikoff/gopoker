package model

import (
	"container/ring"
	"errors"
	"fmt"
	"log"
	"sync"

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
	// DefaultFunds default funds for a player joining the table
	DefaultFunds = 100 * DefaultMinBet
	// MinPlayersToPlay below which the hand cannot start
	MinPlayersToPlay = 2
	// MaxTableSize once reached no more players can sit
	MaxTableSize = 10
	// MaxStandersSize in conjunction with MaxTableSize defines the maximum number of players at a table.
	// There can be up to MaxStandersSize + MaxTableSize standers at a table, but once there are a total
	// of MaxStandersSize + MaxTableSize players at a table no more players may join
	MaxStandersSize = 10
)

type (
	// TableSummary is a summary of a table for listing them in the UI.
	TableSummary struct {
		Name         string
		PlayerCount  int
		StanderCount int
		IsPlaying    bool
	}
	// SerializableTable is the serializable version of a Table, containing all information a client needs.
	SerializableTable struct {
		TableConfig TableConfig
		Players     [MaxTableSize]*Player
		DealerIndex int
		Standers    map[string]*Player
		Hand        *Hand
	}
)

type (
	// Player a player's state at a Table
	Player struct {
		Name          string
		Standing      bool
		WantToStandUp bool
		SeatIndex     int
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
		Standers    map[string]*Player
		Hand        *Hand
		mutex       sync.Mutex
	}

	// TableConfig define nuances of the game played at a Table
	TableConfig struct {
		minBet       int
		defaultFunds int
	}
)

// DefaultConfig creates a default TableConfig.
func DefaultConfig() TableConfig {
	return TableConfig{
		minBet:       DefaultMinBet,
		defaultFunds: DefaultFunds,
	}
}

// NewTable create a new table
func NewTable() *Table {
	table := NewTableWithConfig(
		TableConfig{
			minBet: DefaultMinBet,
		},
	)
	return table
}

// NewTableWithConfig create a new table with custom config
func NewTableWithConfig(tableConfig TableConfig) *Table {
	table := Table{
		TableConfig: tableConfig,
		Standers:    make(map[string]*Player),
	}
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
func (t *Table) playersForHand() (*ring.Ring, Pot) {
	mainPot := SubPot{make(map[*Player]struct{}), 0}
	index := (t.DealerIndex + 1) % len(t.Players)
	var playersPlaying []*Player
	for i := 0; i < len(t.Players); i++ {
		p := t.Players[index]
		if p != nil {
			if p.Funds <= 0 {
				p.Standing = true
				t.Players[index] = nil
			} else {
				playersPlaying = append(playersPlaying, p)
				mainPot.Players[p] = struct{}{}
			}
		}
		index = (index + 1) % len(t.Players)
	}
	if len(playersPlaying) == 0 {
		return nil, Pot{}
	}
	out := ring.New(len(playersPlaying))
	for _, p := range playersPlaying {
		out.Value = p
		out = out.Next()
	}
	return out.Prev(), Pot{MainPot: mainPot, SidePots: []SubPot{}}
}

// SerializableTable generates a SerializableTable copy from the Table.
func (t *Table) SerializableTable(name string) SerializableTable {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	var playersCopy [MaxTableSize]*Player
	for i, p := range t.Players {
		if p != nil {
			pCopy := *p
			playersCopy[i] = &pCopy
			if pCopy.Name != name {
				// Obfuscate the player's hand, if it isn't the current player's.
				pCopy.Hole = nil
			}
		}
	}
	standersCopy := make(map[string]*Player)
	for n, p := range t.Standers {
		pCopy := *p
		standersCopy[n] = &pCopy
	}
	return SerializableTable{
		TableConfig: t.TableConfig,
		Players:     playersCopy,
		DealerIndex: t.DealerIndex,
		Standers:    standersCopy,
		Hand:        t.Hand,
	}
}

// StartHand starts a hand
func (t *Table) StartHand() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.Hand.StartHand()
}

func (t *Table) incrementDealerIndex() error {
	log.Printf("dealer index: %d\n", t.DealerIndex)
	for i := 1; i < len(t.Players); i++ {
		dealerIndex := (i + t.DealerIndex) % len(t.Players)
		p := t.Players[dealerIndex]
		if p != nil && p.Playing && i != t.DealerIndex {
			log.Printf("found player: %s, index: %d", p.Name, dealerIndex)
			t.DealerIndex = dealerIndex
			return nil
		}
	}
	return errors.New("incrementdealerindex: could not find next dealer")
}

// Join stand a player at the table
func (t *Table) Join(p *Player) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if _, ok := t.Standers[p.Name]; ok {
		return fmt.Errorf("duplicate name: %s", p.Name)
	} else if len(t.Players)+len(t.Standers) >= MaxTableSize+MaxStandersSize {
		return fmt.Errorf("too many players %d", len(t.Players)+len(t.Standers))
	}
	t.Standers[p.Name] = p
	p.table = t
	p.Standing = true
	if p.Funds < t.TableConfig.defaultFunds {
		p.Funds = t.TableConfig.defaultFunds
	}
	return nil
}

// SitDown seat a player at the table
func (t *Table) SitDown(p *Player, seat int) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if p.table == t && !p.Standing {
		return fmt.Errorf("Player already sitting")
	} else if p.Funds < t.TableConfig.minBet {
		return fmt.Errorf("Player has insufficient funds to sit (%d < %d)", p.Funds, t.TableConfig.minBet)
	} else if seat >= MaxTableSize {
		return errors.New("Seat, " + fmt.Sprint(seat) +
			" is greater than max table size, " + fmt.Sprint(MaxTableSize))
	} else if t.Players[seat] == nil {
		t.Players[seat] = p
		p.table = t
		p.Standing = false
		p.SeatIndex = seat
		return nil
	}
	return errors.New("seat is occupied, " + fmt.Sprint(seat))
}

// Leave removes a stander at the table
func (t *Table) Leave(p *Player) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if _, ok := t.Standers[p.Name]; ok {
		delete(t.Standers, p.Name)
		p.table = nil
		return nil
	}
	return errors.New("player is not standing at this table")
}

// Leave a player at the next chance
func (p *Player) Leave() error {
	return p.GetTable().Leave(p)
}

// GetPlayers returns the table's players
func (t *Table) GetPlayers() [MaxTableSize]*Player {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.Players
}

// HandleStanders stands up players that want to stand
func (t *Table) HandleStanders() []string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	var newStanders []string
	for i, p := range t.Players {
		if p != nil && p.WantToStandUp {
			t.Players[i].Playing = false
			t.Players[i].Standing = true
			t.Players[i].WantToStandUp = false
			t.Players[i] = nil
			t.Standers[p.Name] = p
			newStanders = append(newStanders, p.Name)
		}
	}
	return newStanders
}

// StandNow stands the player immediately
func (p *Player) StandNow() {
	p.table.mutex.Lock()
	defer p.table.mutex.Unlock()
	p.Playing = false
	p.Standing = true
	p.WantToStandUp = false
	p.table.Players[p.SeatIndex] = nil
	p.table.Standers[p.Name] = p
}

// StandUp a player at the next chance
func (p *Player) StandUp() {
	p.WantToStandUp = true
}

// RoundDone gets whether the round is done
func (t *Table) RoundDone() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.Hand.Round.RoundDone
}

// SetRoundDone sets whether the round is done
func (t *Table) SetRoundDone(d bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.Hand.Round.RoundDone = d
}

// BettingDone gets whether betting is done
func (t *Table) BettingDone() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.Hand.BettingDone
}

// CurrentBetter gets the current better
func (t *Table) CurrentBetter() *Player {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.Hand.Round.BetTurn.Value.(*Player)
}

// HandlePlayerAction handles a player's desired action
func (t *Table) HandlePlayerAction(p *Player, action RoundAction) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.Hand.PlayerAction(p, action)

}

// String player's string
func (p Player) String() string {
	cards := ""
	betAmount := ""
	if p.Playing {
		if len(p.Hole) > 0 {
			cards = ", Cards: "
		}
		for _, c := range p.Hole {
			cards += fmt.Sprint(c) + " "
		}
		betAmount = fmt.Sprintf(", BetAmount: %d", p.BetAmount)
	} else {
		betAmount = ", not playing"
	}
	return fmt.Sprintf("%s, funds: %v%v %s", p.Name, p.Funds, betAmount, cards)
}

// String table's string
func (t *Table) String() string {
	out := fmt.Sprintf("bettingDone: %v, handDone: %v\n", t.Hand.BettingDone, t.Hand.HandDone)
	if len(t.Hand.Board) > 0 {
		out += "Board="
		for _, c := range t.Hand.Board {
			out += fmt.Sprint(c) + " "
		}
		out += "\n"
	}
	for _, pot := range append(t.Hand.Pot.SidePots, t.Hand.Pot.MainPot) {
		if pot.Pot != 0 {
			out += "Pot=" + fmt.Sprint(pot.Pot) +
				", player_count=" + fmt.Sprint(len(pot.Players)) + "\n"
		}
	}
	for i, p := range t.Players {
		seat := "Seat: " + fmt.Sprint(i) + ", " + fmt.Sprint(p)
		out += seat
		if p == RingToPlayer(t.Hand.Round.BetTurn) {
			out += " (B) "
		}
		if p == t.Hand.Dealer() {
			out += " (D) "
		}
		out += "\n"
	}
	return out
}

// GetTable get the player's table
func (p *Player) GetTable() *Table {
	return p.table
}
