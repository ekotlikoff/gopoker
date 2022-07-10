package model

import (
	"fmt"
	"testing"
	"time"
)

func TestNextBetter(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 200}, 0) // dealer
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)  // sb
	table.SitDown(&Player{Name: "Bob", Funds: 200}, 4)  // bb
	table.SitDown(&Player{Name: "Nora", Funds: 200}, 5)
	table.Hand = table.NewHand()
	hand := table.Hand
	if pRing(hand.BetTurn).Name != "Anna" {
		t.Error("expected Anna as initial better")
	}
	hand.nextBetter()
	if hand.RoundDone {
		t.Error("round should not be done")
	}
	if pRing(hand.BetTurn).Name != "Joe" {
		t.Error("expected Bob as next better got", pRing(hand.BetTurn).Name)
	}
}

func TestStartHand(t *testing.T) {
	table := NewTable()
	table.SitDown(&Player{Name: "Anna", Funds: 200}, 0)
	table.SitDown(&Player{Name: "Joe", Funds: 200}, 2)
	table.SitDown(&Player{Name: "Bob", Funds: 200}, 4)
	table.SitDown(&Player{Name: "Nora", Funds: 200}, 5)
	table.Hand = table.NewHand()
	hand := table.Hand
	fmt.Println(hand.StartHand())
	if pRing(hand.FirstToBet).Name != "Nora" {
		t.Error("expected Nora as first better got", pRing(hand.FirstToBet).Name)
	}
	if pRing(hand.BetTurn).Name != "Nora" {
		t.Error("expected Nora as next better got", pRing(hand.BetTurn).Name)
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
	if !hand.RoundDone {
		t.Error("expected rounddone")
	}
}

func TestPlayAllIn(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	table.Players[0].StandUp()
	table.Players[2].StandUp()
	table.Players[2].ActionChan <- RoundAction{Raise, 400}
	table.Players[0].ActionChan <- RoundAction{Call, 400}
	retries := 0
	for table.playing && retries < 5 {
		time.Sleep(time.Millisecond)
		retries++
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	fmt.Println(table)
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestPlayFoldWin(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	table.Players[0].StandUp()
	table.Players[2].StandUp()
	table.Players[2].ActionChan <- RoundAction{Fold, 0}
	fmt.Println(table)
	retries := 0
	for table.playing && retries < 5 {
		time.Sleep(time.Millisecond)
		retries++
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestPlayFoldWithRematch(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	table.Players[2].ActionChan <- RoundAction{Fold, 0}
	table.Players[0].StandUp()
	table.Players[2].StandUp()
	table.Players[0].ActionChan <- RoundAction{Fold, 0}
	fmt.Println(table)
	totalFunds := paul.Funds + leto.Funds
	retries := 0
	for table.playing && retries < 5 {
		time.Sleep(time.Millisecond)
		retries++
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestPlaySimple(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	table.Players[2].ActionChan <- RoundAction{Call, 200}
	table.Players[0].ActionChan <- RoundAction{Call, 200}
	table.Players[2].ActionChan <- RoundAction{Call, 0}
	table.Players[0].ActionChan <- RoundAction{Call, 0}
	table.Players[2].ActionChan <- RoundAction{Call, 0}
	table.Players[0].ActionChan <- RoundAction{Call, 0}
	table.Players[2].ActionChan <- RoundAction{Call, 0}
	table.Players[0].ActionChan <- RoundAction{Call, 0}
	table.Players[0].StandUp()
	table.Players[2].StandUp()
	fmt.Println(table)
	retries := 0
	for table.playing && retries < 5 {
		time.Sleep(time.Millisecond)
		retries++
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestPlayNoPlayerAtSeatZero(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 400)
	table.SitDown(leto, 1)
	paul := NewPlayerWithFunds("Paul", 400)
	table.SitDown(paul, 2)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	table.Players[2].ActionChan <- RoundAction{Call, 200}
	table.Players[1].ActionChan <- RoundAction{Call, 200}
	table.Players[2].ActionChan <- RoundAction{Call, 0}
	table.Players[1].ActionChan <- RoundAction{Call, 0}
	table.Players[2].ActionChan <- RoundAction{Call, 0}
	table.Players[1].ActionChan <- RoundAction{Call, 0}
	table.Players[1].StandUp()
	table.Players[2].StandUp()
	table.Players[2].ActionChan <- RoundAction{Call, 0}
	table.Players[1].ActionChan <- RoundAction{Call, 0}
	fmt.Println(table)
	retries := 0
	for table.playing && retries < 5 {
		time.Sleep(time.Millisecond)
		retries++
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 800 {
		t.Error("expected 800 got", totalFunds)
	}
}

func TestPlayFirstToBetChanges(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Second * 30,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 800)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 800)
	table.SitDown(paul, 2)
	frank := NewPlayerWithFunds("Frank", 800)
	table.SitDown(frank, 3)
	beth := NewPlayerWithFunds("Beth", 800)
	table.SitDown(beth, 4)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	beth.ActionChan <- RoundAction{Call, 200}
	leto.ActionChan <- RoundAction{Call, 200}
	paul.ActionChan <- RoundAction{Call, 200}
	frank.ActionChan <- RoundAction{Call, 200}
	paul.ActionChan <- RoundAction{Call, 200}
	frank.ActionChan <- RoundAction{Call, 200}
	beth.ActionChan <- RoundAction{Call, 200}
	leto.ActionChan <- RoundAction{Call, 200}
	paul.ActionChan <- RoundAction{Call, 0}
	frank.ActionChan <- RoundAction{Raise, 200}
	beth.ActionChan <- RoundAction{Call, 200}
	leto.ActionChan <- RoundAction{Call, 200}
	paul.ActionChan <- RoundAction{Fold, 0}
	paul.StandUp()
	frank.StandUp()
	beth.StandUp()
	leto.StandUp()
	frank.ActionChan <- RoundAction{Call, 0}
	beth.ActionChan <- RoundAction{Call, 0}
	leto.ActionChan <- RoundAction{Call, 0}
	fmt.Println(table)
	retries := 0
	for table.playing && retries < 5 {
		time.Sleep(time.Millisecond)
		retries++
	}
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds + frank.Funds + beth.Funds
	if totalFunds != 3200 {
		t.Error("expected 3200 got", totalFunds)
	}
}

func TestTimeoutIsFold(t *testing.T) {
	table := NewTableWithConfig(TableConfig{
		minBet: DefaultMinBet, timeToBet: time.Millisecond * 2,
		secondsBetweenHands: time.Second * 0,
	})
	leto := NewPlayerWithFunds("Leto", 800)
	table.SitDown(leto, 0)
	paul := NewPlayerWithFunds("Paul", 800)
	table.SitDown(paul, 2)
	go func() {
		err := table.Play()
		fmt.Println(err)
	}()
	leto.StandUp()
	paul.StandUp()
	time.Sleep(time.Millisecond * 3)
	fmt.Println(table)
	if table.playing {
		t.Error("table should be done playing")
	}
	totalFunds := paul.Funds + leto.Funds
	if totalFunds != 1600 {
		t.Error("expected 3200 got", totalFunds)
	}
}
