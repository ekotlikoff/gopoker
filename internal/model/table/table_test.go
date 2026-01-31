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
	if table.Hand.Dealer().Name != "Anna" {
		t.Error("expected Anna as dealer got", table.Hand.Dealer().Name)
	}
	if table.Hand.FirstToBet != nil {
		t.Error("expected nil first better got", RingToPlayer(table.Hand.FirstToBet).Name)
	}
	if RingToPlayer(table.Hand.Round.BetTurn).Name != "Nora" {
		t.Error("expected Nora as next better got", RingToPlayer(table.Hand.Round.BetTurn).Name)
	}
	if table.Players[0].BetAmount != 0 {
		t.Error("blinds not taken correctly", table.Players[0].BetAmount)
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
	if table.Players[0].BetAmount != 100 {
		t.Error("blinds not taken correctly", table.Players[2].BetAmount)
	}
	if table.Players[2].BetAmount != 200 {
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
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200}, true)
	err := table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 200}, true)
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
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200}, true)
	table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 200}, true)
	err := table.Hand.PlayerAction(table.Players[3], RoundAction{Raise, 400}, true)
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
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 200}, true)
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	_, err := table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := table.Players[0].Funds + table.Players[2].Funds + table.Players[3].Funds
	if totalFunds != 600 {
		t.Error("expected 600 got", totalFunds)
	}
}

func TestAllInCall(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 500)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Raise, 400}, true)
	table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 400}, true)
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	fmt.Println(table)
	_, err := table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 900 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestAllIn(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 1000)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 600)
	table.SitDown(paul, 2)
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	table.Hand.PlayerAction(table.Players[0], RoundAction{Raise, 400}, true)
	table.Hand.PlayerAction(table.Players[2], RoundAction{AllIn, 600}, true)
	err := table.Hand.Deal()
	if err == nil {
		t.Error(err)
	}
	table.Hand.PlayerAction(table.Players[0], RoundAction{Call, 600}, true)
	table.Hand.Deal()
	table.Hand.Deal()
	table.Hand.Deal()
	fmt.Println(table)
	_, err = table.Hand.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 1600 {
		t.Error("expected 1600 got", totalFunds)
	}
}

func TestFoldWin(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.NewHand()
	table.Hand.StartHand()
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	_, err = table.Hand.FinishHand()
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
		MinBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(table)
	_, _, err = table.FinishHand()
	if err != nil {
		t.Error(err)
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
	table.NewHand()
	fmt.Println(table.Hand.StartHand())
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(table)
	_, _, err = table.FinishHand()
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
		MinBet: DefaultMinBet,
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
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{AllIn, 400}, true)
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[3], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	if table.Hand.Players.Len() != 1 {
		t.Error("expected 1 players, got", table.Hand.Players.Len())
	}
	_, _, err = table.FinishHand()
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

func TestFirstBetterFolds(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: DefaultMinBet,
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
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 200}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[3], RoundAction{Call, 200}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.Deal()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
}

func TestRaiseAndCall(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: DefaultMinBet,
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
	err := table.Hand.PlayerAction(table.Players[0], RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 200}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[3], RoundAction{Raise, 400}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	err = table.Hand.PlayerAction(table.Players[2], RoundAction{Call, 400}, true)
	if err != nil {
		t.Log(table)
		t.Error(err)
		t.Errorf("expected the round of betting to continue after a raise, got error: %v", err)
	}
	err = table.Hand.Deal()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
}

func TestNextDealer(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: DefaultMinBet,
	})
	leto := NewPlayerWithFunds("Leto", 800)
	table.SitDown(leto, 1)
	paul := NewPlayerWithFunds("Paul", 800)
	table.SitDown(paul, 2)
	frank := NewPlayerWithFunds("Frank", 800)
	table.SitDown(frank, 3)
	table.NewHand()
	table.Hand.StartHand()
	if table.Dealer() != leto {
		t.Errorf("expected Leto as the better, got %s", table.Dealer().Name)
	}
	if table.Hand.Players.Len() != 3 {
		t.Error("expected 3 players, got", table.Hand.Players.Len())
	}
	err := table.Hand.PlayerAction(leto, RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	err = table.Hand.PlayerAction(paul, RoundAction{Fold, 0}, true)
	if err != nil {
		t.Error(err)
	}
	_, _, err = table.FinishHand()
	if err != nil {
		t.Log(table)
		t.Error(err)
	}
	table.NewHand()
	table.Hand.StartHand()
	if table.Hand.Players.Len() != 3 {
		t.Error("expected 3 players, got", table.Hand.Players.Len())
	}
	if table.Dealer() != paul {
		t.Errorf("expected Paul as the better, got %s", table.Dealer().Name)
	}
}

func TestSidePot(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: 100,
	})
	anna := NewPlayerWithFunds("Anna", 1000)
	table.SitDown(anna, 0)
	joe := NewPlayerWithFunds("Joe", 1500)
	table.SitDown(joe, 2)
	bob := NewPlayerWithFunds("Bob", 500)
	table.SitDown(bob, 4)

	table.NewHand()
	err := table.Hand.StartHand()
	if err != nil {
		t.Fatal(err)
	}

	// Pre-flop
	if err := table.Hand.PlayerAction(anna, RoundAction{Call, 200}, true); err != nil {
		t.Fatal(err)
	}
	if err := table.Hand.PlayerAction(joe, RoundAction{Call, 200}, true); err != nil {
		t.Fatal(err)
	}

	if err := table.Hand.PlayerAction(bob, RoundAction{AllIn, 500}, true); err != nil {
		t.Fatal(err)
	}
	if err := table.Hand.PlayerAction(anna, RoundAction{Call, 500}, true); err != nil {
		t.Fatal(err)
	}
	if err := table.Hand.PlayerAction(joe, RoundAction{Call, 500}, true); err != nil {
		t.Fatal(err)
	}

	if err := table.Hand.Deal(); err != nil {
		t.Fatal(err)
	}

	// Post-flop
	if err := table.Hand.PlayerAction(joe, RoundAction{Raise, 200}, true); err != nil {
		t.Fatal(err)
	}
	if err := table.Hand.PlayerAction(anna, RoundAction{Call, 200}, true); err != nil {
		t.Fatal(err)
	}

	if len(table.Hand.Pot.SidePots) != 1 {
		t.Fatalf("expected 1 side pot, got %d", len(table.Hand.Pot.SidePots))
	}
}

func TestSidePotAfterFirstRound(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		MinBet: 100,
	})
	anna := NewPlayerWithFunds("Anna", 1000)
	table.SitDown(anna, 0)
	joe := NewPlayerWithFunds("Joe", 300)
	table.SitDown(joe, 2)
	bob := NewPlayerWithFunds("Bob", 1500)
	table.SitDown(bob, 4)

	table.NewHand()
	err := table.Hand.StartHand()
	if err != nil {
		t.Fatal(err)
	}

	// Pre-flop
	// UTG is anna
	// Small blind is Joe
	// Big blind is bob

	// Anna calls
	if err := table.Hand.PlayerAction(anna, RoundAction{Call, 200}, true); err != nil {
		t.Fatal(err)
	}

	// Joe goes all-in for 300
	if err := table.Hand.PlayerAction(joe, RoundAction{AllIn, 300}, true); err != nil {
		t.Fatal(err)
	}

	// Bob calls 300
	if err := table.Hand.PlayerAction(bob, RoundAction{Call, 300}, true); err != nil {
		t.Fatal(err)
	}

	// Anna calls 300
	if err := table.Hand.PlayerAction(anna, RoundAction{Call, 300}, true); err != nil {
		t.Fatal(err)
	}

	if len(table.Hand.Pot.SidePots) != 1 {
		t.Fatalf("expected 1 side pot, got %d", len(table.Hand.Pot.SidePots))
	}

	if table.Hand.Pot.SidePots[0].Pot != 900 {
		t.Errorf("expected side pot to be 900, got %d", table.Hand.Pot.SidePots[0].Pot)
	}
	if len(table.Hand.Pot.SidePots[0].Players) != 3 {
		t.Errorf("expected 3 players in side pot, got %d", len(table.Hand.Pot.SidePots[0].Players))
	}
	_, ok := table.Hand.Pot.SidePots[0].Players["Anna"]
	if !ok {
		t.Errorf("expected Anna in side pot")
	}
	_, ok = table.Hand.Pot.SidePots[0].Players["Joe"]
	if !ok {
		t.Errorf("expected Joe in side pot")
	}
	_, ok = table.Hand.Pot.SidePots[0].Players["Bob"]
	if !ok {
		t.Errorf("expected Bob in side pot")
	}

	if table.Hand.Pot.MainPot.Pot != 0 {
		t.Errorf("expected main pot to be 0, got %d", table.Hand.Pot.MainPot.Pot)
	}
	if len(table.Hand.Pot.MainPot.Players) != 0 {
		t.Errorf("expected 0 players in main pot, got %d", len(table.Hand.Pot.MainPot.Players))
	}
}
