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
		// If dealing is still needed but no more betting
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

func pRing(ring *ring.Ring) *Player {
	return ring.Value.(*Player)
}

func (hand *Hand) validateBlinds() error {
	lbValid := hand.SmallBlind().Funds >= hand.TableConfig.minBet/2
	bbValid := hand.BigBlind().Funds >= hand.TableConfig.minBet
	if !lbValid || !bbValid {
		errStr := "failed to validate blinds, lbFunds=%d bbFunds=%d minBet=%d"
		return fmt.Errorf(errStr, hand.SmallBlind().Funds, hand.BigBlind().Funds,
			hand.TableConfig.minBet)
	}
	return nil
}

// StartHand start a hand
func (hand *Hand) StartHand() error {
	if hand.Players.Len() < MinPlayersToPlay {
		return errors.New("starthand: insufficient players to start hand")
	}
	if err := hand.validateBlinds(); err != nil {
		return fmt.Errorf("starthand: %w", err)
	}
	hand.HandDone = false
	hand.Deck.Shuffle()
	player := hand.Players
	for i := 0; i < hand.Players.Len(); i++ {
		pRing(player).Playing = true
		hand.dealHole(pRing(player))
		player = player.Next()
	}
	hand.startBets()
	return nil
}

// Dealer is the dealer of the hand
func (hand *Hand) Dealer() *Player {
	return pRing(hand.Players)
}

// SmallBlind is the small blind of the hand
func (hand *Hand) SmallBlind() *Player {
	return pRing(hand.Players.Next())
}

// BigBlind is the big blind of the hand
func (hand *Hand) BigBlind() *Player {
	return pRing(hand.Players.Next().Next())
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
		if hand.BetterCount() < 1 {
			hand.BettingDone = true
			log.Println("Player allin ended betting")
		}
	}
	player.Funds -= (bet - player.BetAmount)
	player.BetAmount = bet
	return nil
}

// PlayerAction handles a player action
func (hand *Hand) PlayerAction(
	player *Player, action RoundAction) error {
	if pRing(hand.Round.BetTurn) != player || hand.Round.RoundDone {
		return errors.New("it's not your turn to bet")
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
	hand.nextBetter()
	return nil
}

func (hand *Hand) nextBetter() {
	if hand.Round.RoundDone {
		log.Println("Skipping nextbetter because round is done")
		return
	}
	better := hand.Round.BetTurn.Next()
	for i := 0; i < better.Len(); i++ {
		player := pRing(better)
		if hand.FirstToBet != nil && player == pRing(hand.FirstToBet) {
			log.Println("Back to firsttobet, ending the round", player.Name)
			break
		} else if !player.AllIn {
			log.Println("Found better", pRing(better).Name)
			hand.Round.BetTurn = better
			return
		}
		better = better.Next()
	}
	hand.Round.RoundDone = true
}

func (hand *Hand) playerFold() {
	player := pRing(hand.Round.BetTurn)
	player.Hole = []poker.Card{}
	hand.Pot.MainPot.Pot += player.BetAmount
	player.BetAmount = 0
	for _, pot := range append(hand.Pot.SidePots, hand.Pot.MainPot) {
		delete(pot.Players, player)
	}
	if hand.Players.Len() < 3 {
		hand.HandDone = true
		log.Println("Player fold ended the hand")
	} else if hand.BetterCount() < 3 {
		hand.BettingDone = true
		log.Println("Player fold ended betting")
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
		player := pRing(better)
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
	hand.Round.RoundDone = false
	hand.startBets()
	return nil
}

func (hand *Hand) dealHole(player *Player) {
	player.Hole = hand.Deck.Draw(2)
}

// String the hand's string
func (hand *Hand) String() string {
	out := ""
	if hand.Round.RoundDone {
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
		if p == pRing(hand.Round.BetTurn) {
			out += " (B) "
		}
		if p == hand.Dealer() {
			out += " (D) "
		}
		out += "\n"
	})
	return out
}
