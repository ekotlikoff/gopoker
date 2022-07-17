package model

import (
	"container/ring"
	"errors"
	"fmt"
	"log"

	"github.com/chehsunliu/poker"
)

type (
	// Hand can be played and winners will be identified
	Hand struct {
		// TableConfig defines nuances of play
		TableConfig TableConfig
		// Deck of cards
		Deck poker.Deck
		// Board shared cards
		Board []poker.Card
		// Round is the current round of betting
		Round *Round
		// Players in the hand
		Players *ring.Ring
		// Pot of winnings
		Pot Pot
		// FirstToBet bets first
		FirstToBet *ring.Ring
		// If dealing is still needed but no more betting, e.g. players are all in
		BettingDone bool
		// If no more dealing is needed for the hand
		HandDone bool
	}

	// Round is a cycle of betting, there are 4 in a hand: pre-flop, flop, turn, river
	Round struct {
		// BetTurn is betting next
		BetTurn *ring.Ring
		// CurrentBet is the amount to call
		CurrentBet int
		// If the round of betting is done
		RoundDone bool
	}
)

// NewHand create a hand
func (table *Table) NewHand() *Hand {
	if table.Players[table.DealerIndex] == nil {
		table.incrementDealerIndex()
	}
	players, pot := table.playersForHand()
	return &Hand{
		Deck:        poker.Deck{},
		TableConfig: table.TableConfig,
		Players:     players,
		Pot:         pot,
	}
}

// FinishHand ends a hand and handles standing players up
func (table *Table) FinishHand() error {
	err := table.Hand.FinishHand()
	if err != nil {
		return err
	}
	// Clear player holes and handle standups
	table.Hand.Players.Do(func(p interface{}) {
		player := p.(*Player)
		player.Hole = []poker.Card{}
		if player.Funds == 0 || player.WantToStandUp {
			table.standUp(player)
		}
	})
	return table.incrementDealerIndex()
}

// RingToPlayer converts from a ring buffer to a player
func RingToPlayer(ring *ring.Ring) *Player {
	return ring.Value.(*Player)
}

// StartHand start a hand
func (hand *Hand) StartHand() error {
	if hand.Players.Len() < MinPlayersToPlay {
		return errors.New("starthand: insufficient players to start hand")
	}
	hand.HandDone = false
	hand.Deck.Shuffle()
	player := hand.Players
	for i := 0; i < hand.Players.Len(); i++ {
		RingToPlayer(player).Playing = true
		hand.dealHole(RingToPlayer(player))
		player = player.Next()
	}
	hand.startBets()
	return nil
}

// Dealer is the dealer of the hand
func (hand *Hand) Dealer() *Player {
	return RingToPlayer(hand.Players)
}

// SmallBlind is the small blind of the hand
func (hand *Hand) SmallBlind() *Player {
	return RingToPlayer(hand.Players.Next())
}

// BigBlind is the big blind of the hand
func (hand *Hand) BigBlind() *Player {
	return RingToPlayer(hand.Players.Next().Next())
}

func (hand *Hand) takeBlinds() {
	hand.SmallBlind().Funds -= hand.TableConfig.minBet / 2
	hand.SmallBlind().BetAmount = hand.TableConfig.minBet / 2
	hand.BigBlind().Funds -= hand.TableConfig.minBet
	hand.BigBlind().BetAmount = hand.TableConfig.minBet
	if hand.SmallBlind().Funds == 0 {
		hand.SmallBlind().AllIn = true
	}
	if hand.BigBlind().Funds == 0 {
		hand.BigBlind().AllIn = true
	}
	if (hand.SmallBlind().AllIn || hand.BigBlind().AllIn) &&
		hand.Players.Len() == 2 {
		hand.Round.RoundDone = true
		hand.BettingDone = true
	}
}

func (hand *Hand) startBets() {
	hand.FirstToBet = nil
	hand.Round = &Round{
		BetTurn: hand.Players,
	}
	log.Println("board length", len(hand.Board))
	if len(hand.Board) == 0 {
		hand.Round.CurrentBet = hand.TableConfig.minBet
		hand.takeBlinds()
		hand.Round.BetTurn = hand.Players.Next().Next()
	} else {
		hand.Round.CurrentBet = 0
		hand.Round.BetTurn = hand.Players
	}
	hand.nextBetter()
	hand.FirstToBet = hand.Round.BetTurn
}

func (hand *Hand) playerBet(player *Player, bet int) error {
	allIn := bet == player.Funds+player.BetAmount
	raise := bet > hand.Round.CurrentBet
	if bet-player.BetAmount > player.Funds {
		return errors.New("insufficient funds")
	} else if bet < hand.Round.CurrentBet && !allIn {
		return errors.New("insufficient bet")
	} else if raise {
		if bet-hand.Round.CurrentBet < hand.TableConfig.minBet {
			return errors.New("cannot raise less than the big blind")
		}
		hand.Round.CurrentBet = bet
		hand.FirstToBet = hand.Round.BetTurn
	}
	if allIn {
		player.AllIn = true
	}
	player.Funds -= (bet - player.BetAmount)
	player.BetAmount = bet
	return nil
}

// PlayerAction handles a player action
func (hand *Hand) PlayerAction(
	player *Player, action RoundAction) error {
	if hand.Round == nil {
		return errors.New("playeraction: there is no round")
	} else if RingToPlayer(hand.Round.BetTurn) != player || hand.Round.RoundDone {
		return errors.New("playeraction: it's not your turn to bet")
	}
	var err error
	switch action.actionType {
	case Call:
		err = hand.playerBet(player, hand.Round.CurrentBet)
	case AllIn:
		if player.Funds != action.bet-player.BetAmount {
			return fmt.Errorf("playeraction: this is not an all in, funds=%d bet=%d",
				player.Funds, action.bet)
		}
		err = hand.playerBet(player, action.bet)
	case Raise:
		err = hand.playerBet(player, action.bet)
	case Fold:
		hand.playerFold()
	}
	if err != nil {
		return err
	}
	hand.checkForBettingCompletion()
	hand.nextBetter()
	if hand.Round.RoundDone {
		hand.createPots()
	}
	return nil
}

func (hand *Hand) checkForBettingCompletion() {
	if hand.Players.Len() == 1 {
		// If there is only 1 player left, the hand is done
		hand.HandDone = true
		hand.Round.RoundDone = true
	} else if hand.BetterCount() <= 1 {
		// If there is only 1 player left betting dealing must continue
		hand.BettingDone = true
	}
}

func (hand *Hand) nextBetter() {
	if hand.Round.RoundDone {
		log.Println("Skipping nextbetter because round is done")
		return
	}
	better := hand.Round.BetTurn.Next()
	for i := 0; i < better.Len(); i++ {
		player := RingToPlayer(better)
		if hand.FirstToBet != nil && player == RingToPlayer(hand.FirstToBet) {
			log.Println("Back to firsttobet, ending the round", player.Name)
			break
		} else if !player.AllIn {
			log.Println("Found better", RingToPlayer(better).Name)
			hand.Round.BetTurn = better
			return
		}
		better = better.Next()
	}
	hand.Round.RoundDone = true
}

func (hand *Hand) playerFold() {
	player := RingToPlayer(hand.Round.BetTurn)
	player.Hole = []poker.Card{}
	hand.Pot.MainPot.Pot += player.BetAmount
	player.BetAmount = 0
	for _, pot := range append(hand.Pot.SidePots, hand.Pot.MainPot) {
		delete(pot.Players, player)
	}
	if hand.Round.BetTurn == hand.Players {
		hand.Players = hand.Players.Prev()
	}
	hand.Round.BetTurn = hand.Round.BetTurn.Prev()
	hand.Round.BetTurn.Unlink(1)
}

// BetterCount determines the number of betters
func (hand *Hand) BetterCount() int {
	betters := 0
	better := hand.Round.BetTurn
	for i := 0; i < better.Len(); i++ {
		player := RingToPlayer(better)
		if !player.AllIn {
			betters++
		}
		better = better.Next()
	}
	return betters
}

// Deal adds shared cards on the board
func (hand *Hand) Deal() error {
	if !hand.Round.RoundDone {
		return errors.New("deal: currently betting")
	} else if len(hand.Board) >= 5 {
		return errors.New("dealing is done")
	}
	cardsToDraw := 3
	if len(hand.Board) >= 3 {
		cardsToDraw = 1
	}
	hand.Board = append(hand.Board, hand.Deck.Draw(cardsToDraw)...)
	if !hand.BettingDone {
		hand.Round.RoundDone = false
		hand.startBets()
	} else {
		hand.Round.RoundDone = true
		hand.HandDone = len(hand.Board) == 5
	}
	return nil
}

// FinishHand is called when all betting is complete and the pot should be
// distributed.
func (hand *Hand) FinishHand() error {
	if hand.Round == nil {
		return errors.New("finishhand: there is no round")
	} else if !hand.Round.RoundDone || !hand.HandDone {
		return errors.New("finishhand: table is currently betting")
	}
	log.Println("Distributing pots")
	playerRanking := hand.getPlayerRanking()
	hand.distributePots(playerRanking)
	// Clear board
	hand.Board = []poker.Card{}
	return nil
}

func (hand *Hand) dealHole(player *Player) {
	player.Hole = hand.Deck.Draw(2)
}

// String the hand's string
func (hand *Hand) String() string {
	out := ""
	if hand.Round != nil && hand.Round.RoundDone {
		out += "RoundOver\n"
	}
	if len(hand.Board) > 0 {
		out += "Board="
		for _, c := range hand.Board {
			out += fmt.Sprint(c) + " "
		}
		out += "\n"
	}
	for _, pot := range append(hand.Pot.SidePots, hand.Pot.MainPot) {
		if pot.Pot != 0 {
			out += "Pot=" + fmt.Sprint(pot.Pot) +
				", player_count=" + fmt.Sprint(len(pot.Players)) + "\n"
		}
	}
	hand.Players.Do(func(v interface{}) {
		p := v.(*Player)
		out += fmt.Sprint(p)
		if hand.Round != nil && p == RingToPlayer(hand.Round.BetTurn) {
			out += " (B) "
		}
		if p == hand.Dealer() {
			out += " (D) "
		}
		out += "\n"
	})
	return out
}
