package model

import (
	"fmt"
	"testing"
)

func TestStartHand(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 200}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.SitDown(&Player{Name: "Bob", Funds: 200}, 4)
	table.SitDown(&Player{Name: "Nora", Funds: 200}, 5)
	table.NewHand()
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
	fmt.Println(table.SitDown(&Player{Name: "Anna", Funds: 200}, 0))
	fmt.Println(table.SitDown(&Player{Name: "Joe", Funds: 300}, 2))
	table.Players[0].Funds = 100
	table.NewHand()
	hand := table.Hand
	fmt.Println(hand)
	fmt.Println(hand.StartHand())
	if table.Players[2].BetAmount != 200 {
		t.Error("blinds not taken correctly", table.Players[2].BetAmount)
	}
	if table.Players[0].BetAmount != 100 {
		t.Error("blinds not taken correctly", table.Players[0].BetAmount)
	}
	if !hand.Round.RoundDone {
		t.Error("expected rounddone")
	}
}

func TestHeadsUp(t *testing.T) {
	// When there are two players, the dealer posts the small blind.
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 300}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 400}, 2)
	table.NewHand()
	hand := table.Hand
	fmt.Println(hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200})
	err := table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 200})
	if err != nil {
		t.Error(err)
	}
}

func TestBigBlindGetsToRaise(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 300}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.SitDown(&Player{Name: "Baker", Funds: 400}, 3)
	table.NewHand()
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
	table.NewHand()
	hand := table.Hand
	fmt.Println(hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200})
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	err, _ := table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := table.Players[0].Funds + table.Players[2].Funds + table.Players[3].Funds
	if totalFunds != 600 {
		t.Error("expected 600 got", totalFunds)
	}
}

func TestAllIn(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 500)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Raise, 400})
	table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 400})
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	fmt.Println(table)
	err, _ := table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 900 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestFoldWin(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.NewHand()
	table.Hand.StartHand()
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0})
	if err != nil {
		t.Error(err)
	}
	err, _ = table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestRematch(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0})
	if err != nil {
		t.Error(err)
	}
	fmt.Println(table)
	err, _ = table.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Fold, 0})
	if err != nil {
		t.Error(err)
	}
	fmt.Println(table)
	err, _ = table.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds = paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestFold(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	frank := NewPlayerWithFunds("Frank", 400)
	table.SitDown(frank, 3)
	table.NewHand()
	table.Hand.StartHand()
	if table.Hand.Players.Len() != 3 {
		t.Error("expected 3 players, got", table.Hand.Players.Len())
	}
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{AllIn, 400})
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Fold, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[3], RoundAction{Fold, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	if table.Hand.Players.Len() != 1 {
		t.Error("expected 1 players, got", table.Hand.Players.Len())
	}
	err, _ = table.FinishHand()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds + frank.Funds
	if totalFunds != 1200 {
		t.Error("expected 1200 got", totalFunds)
	}
	table.NewHand()
}

func TestRematchPlayerOutOfFunds(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 800)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 800)
	table.SitDown(paul, 2)
	frank := NewPlayerWithFunds("Frank", 800)
	table.SitDown(frank, 3)
	table.NewHand()
	table.Hand.StartHand()
	table.Players[3].Funds = 0
	if table.Hand.Players.Len() != 3 {
		t.Error("expected 3 players, got", table.Hand.Players.Len())
	}
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Raise, 400})
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 400})
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[3], RoundAction{Fold, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.Deal()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.Deal()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.Deal()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 0})
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err, _ = table.FinishHand()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	table.NewHand()
	if table.Hand.Players.Len() != 2 {
		t.Error("expected 2 players, got", table.Hand.Players.Len())
	}
}
