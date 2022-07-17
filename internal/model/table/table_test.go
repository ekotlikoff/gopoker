package model

import (
	"fmt"
	"testing"
	"time"
)

func TestStartHand(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 200}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.SitDown(&Player{Name: "Bob", Funds: 200}, 4)
	table.SitDown(&Player{Name: "Nora", Funds: 200}, 5)
	table.Hand = table.NewHand()
	err := table.Hand.StartHand()
	if err != nil {
		t.Error(err)
	}
	if RingToPlayer(table.Hand.FirstToBet).Name != "Nora" {
		t.Error("expected Nora as first better got", RingToPlayer(table.Hand.FirstToBet).Name)
	}
	if RingToPlayer(table.Hand.Round.BetTurn).Name != "Nora" {
		t.Error("expected Nora as next better got", RingToPlayer(table.Hand.Round.BetTurn).Name)
	}
	if table.Players[2].BetAmount != 100 {
		t.Error("blinds not taken correctly", table.Players[2].BetAmount)
	}
	if table.Players[4].BetAmount != 200 {
		t.Error("blinds not taken correctly", table.Players[4].BetAmount)
	}
}

func TestStartHandAllInSmallBlind(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 300}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.Players[2].Funds = 100
	table.Hand = table.NewHand()
	hand := table.Hand
	fmt.Println(hand.StartHand())
	if table.Players[2].BetAmount != 100 {
		t.Error("blinds not taken correctly", table.Players[2].BetAmount)
	}
	if table.Players[0].BetAmount != 200 {
		t.Error("blinds not taken correctly", table.Players[4].BetAmount)
	}
	if !hand.Round.RoundDone {
		t.Error("expected rounddone")
	}
}

func TestBigBlindGetsToRaise(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 300}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.SitDown(&Player{Name: "Baker", Funds: 400}, 3)
	table.Hand = table.NewHand()
	hand := table.Hand
	fmt.Println(hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200})
	table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 200})
	err := table.Hand.PlayerAction(table.Players[3], RoundAction{Raise, 400})
	if err != nil {
		t.Error(err)
	}
}

func TestAllInSmallBlind(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 300}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.SitDown(&Player{Name: "Baker", Funds: 200}, 3)
	table.Players[2].Funds = 100
	table.Hand = table.NewHand()
	hand := table.Hand
	fmt.Println(hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200})
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	err := table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := table.Players[0].Funds + table.Players[2].Funds + table.Players[3].Funds
	if totalFunds != 600 {
		t.Error("expected 600 got", totalFunds)
	}
}

func TestAllIn(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 500)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.Hand = table.NewHand()
	fmt.Println(table.Hand.StartHand())
	table.Hand.PlayerAction(table.Players[2], RoundAction{Raise, 400})
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 400})
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	fmt.Println(table)
	err := table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 900 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestFoldWin(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.Hand = table.NewHand()
	table.Hand.StartHand()
	err := table.Hand.PlayerAction(table.Players[2], RoundAction{Fold, 0})
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	if table.playing {
		t.Log(table)
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestRematch(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.Hand = table.NewHand()
	fmt.Println(table.Hand.StartHand())
	err := table.Hand.PlayerAction(table.Players[2], RoundAction{Fold, 0})
	if err != nil {
		t.Error(err)
	}
	fmt.Println(table)
	err = table.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if table.playing {
		t.Error("table should be done playing")
	}
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
	table.Hand = table.NewHand()
	fmt.Println(table.Hand.StartHand())
	err = table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0})
	if err != nil {
		t.Error(err)
	}
	fmt.Println(table)
	err = table.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds = paul.Funds + leto.Funds
	if table.playing {
		t.Error("table should be done playing")
	}
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestRematchPlayerOutOfFunds(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	frank := NewPlayerWithFunds("Frank", 400)
	table.SitDown(frank, 3)
	table.Hand = table.NewHand()
	table.Hand.StartHand()
	if table.Hand.Players.Len() != 3 {
		t.Error("expected 3 players, got", table.Hand.Players.Len())
	}
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{AllIn, 400})
	if err != nil {
		t.Error(err)
	}
	table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 400})
	err = table.Hand.PlayerAction(table.Players[3], RoundAction{Fold, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	err = table.FinishHand()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds + frank.Funds
	if table.playing {
		t.Error("table should be done playing")
	}
	if totalFunds != 1200 {
		t.Error("expected 1200 got", totalFunds)
	}
	table.Hand = table.NewHand()
	if table.Hand.Players.Len() != 2 {
		t.Error("expected 2 players, got", table.Hand.Players.Len())
	}
}

func TestRematchNoPlayersLeft(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.Hand = table.NewHand()
	fmt.Println(table.Hand.StartHand())
	table.Hand.PlayerAction(table.Players[2], RoundAction{AllIn, 400})
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 400})
	if err != nil {
		t.Error(err)
	}
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	table.FinishHand()
	totalFunds := paul.Funds + leto.Funds
	if table.playing {
		t.Error("table should be done playing")
	}
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
	table.Hand = table.NewHand()
	err = table.Hand.StartHand()
	if err == nil {
		t.Error("expected error starting next hand, got no error")
	}
}
