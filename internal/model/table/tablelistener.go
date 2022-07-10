package model

import (
	"context"
	"errors"
	"log"
	"time"
)

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
		if err := table.IncrementDealerIndex(); err != nil {
			log.Println(err)
			table.playing = false
			return err
		}
	}
}

func (hand *Hand) ListenForPlayerActions() {
	for !hand.RoundDone && !hand.BettingDone && !hand.HandDone {
		success := false
		player := pRing(hand.BetTurn)
		timeRemaining := hand.TableConfig.timeToBet
		for !success {
			ctx, cancel := context.WithTimeout(context.Background(), timeRemaining)
			defer cancel()
			t := time.Now()
			err := hand.PlayerAction(player, getPlayerAction(player, ctx))
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
	hand.RoundDone = true
}

func getPlayerAction(player *Player, ctx context.Context) Action {
	log.Println("Waiting for action from", player.Name)
	action := Action{actionType: Fold}
	select {
	case action = <-player.ActionChan:
	case <-ctx.Done():
		log.Println(player.Name, "timed out, folding")
	}
	return action
}