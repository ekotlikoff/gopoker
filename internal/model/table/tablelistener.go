package model

import (
	"context"
	"errors"
	"log"
	"time"
)

// Play rounds at the table
func (table *Table) Play() error {
	if table.playing {
		return errors.New("play: table already playing")
	}
	table.playing = true
	for {
		table.Hand = table.NewHand()
		log.Println("Dealing next hand, dealer is", pRing(table.Hand.Players).Name)
		if err := table.Hand.StartHand(); err != nil {
			table.playing = false
			return err
		}
		table.Hand.ListenForPlayerActions()
		for !table.Hand.HandDone {
			table.Hand.Deal()
			table.Hand.ListenForPlayerActions()
			if len(table.Hand.Board) == 5 {
				table.Hand.HandDone = true
			}
		}
		if err := table.Hand.FinishHand(); err != nil {
			log.Println(err)
			table.playing = false
			return err
		}
		time.Sleep(time.Second * table.TableConfig.secondsBetweenHands)
		for _, p := range table.Players {
			if p != nil && p.WantToStandUp {
				table.standUp(p)
			}
		}
		if err := table.incrementDealerIndex(); err != nil {
			log.Println(err)
			table.playing = false
			return err
		}
	}
}

// ListenForPlayerActions get each player's action for the round of bets
func (hand *Hand) ListenForPlayerActions() {
	for !hand.Round.RoundDone && !hand.BettingDone && !hand.HandDone {
		success := false
		player := pRing(hand.Round.BetTurn)
		timeRemaining := hand.TableConfig.timeToBet
		for !success {
			ctx, cancel := context.WithTimeout(context.Background(), timeRemaining)
			defer cancel()
			t := time.Now()
			err := hand.PlayerAction(player, getPlayerAction(ctx, player))
			timeRemaining -= time.Since(t)
			if err == nil {
				success = true
			} else {
				log.Println(err)
			}
		}
		log.Println(player.Name, "made their bet")
	}
	hand.createPots()
	log.Println("Round of betting is done")
	hand.Round.RoundDone = true
}

func getPlayerAction(ctx context.Context, player *Player) RoundAction {
	log.Println("Waiting for action from", player.Name)
	action := RoundAction{actionType: Fold}
	select {
	case action = <-player.ActionChan:
	case <-ctx.Done():
		log.Println(player.Name, "timed out, folding")
	}
	return action
}
