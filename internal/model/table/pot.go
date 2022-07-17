package model

import (
	"sort"

	"github.com/chehsunliu/poker"
)

type (
	// Pot is the money in the Hand that is yet to be distributed
	Pot struct {
		MainPot  SubPot
		SidePots []SubPot
	}

	// SubPot is one of potentially multiple pots and identifies the players in the running to win it
	SubPot struct {
		Players map[*Player]struct{}
		Pot     int
	}
)

func (hand *Hand) createPots() {
	if hand.Round.CurrentBet == 0 {
		return
	}
	playersAscBet := []*Player{}
	hand.Players.Do(func(p interface{}) {
		if p.(*Player).BetAmount > 0 {
			playersAscBet = append(playersAscBet, p.(*Player))
		}
	})
	sort.Slice(playersAscBet, func(i, j int) bool {
		return playersAscBet[i].BetAmount < playersAscBet[j].BetAmount
	})
	bet := hand.Round.CurrentBet
	for i, player := range playersAscBet {
		if player.BetAmount < bet {
			hand.createSidePot(player, playersAscBet, i)
			bet = hand.Round.CurrentBet - player.BetAmount
		} else {
			hand.Pot.MainPot.Pot += player.BetAmount
			hand.Pot.MainPot.Players[player] = struct{}{}
		}
		player.BetAmount = 0
	}
	hand.Round.CurrentBet = 0
}

func (hand *Hand) createSidePot(player *Player, playersAscBet []*Player, i int) {
	// Take bet amt out of everyone's funds, add to mainpot, move mainpot
	// to a sidepot, and create a new mainpot
	hand.Pot.MainPot.Pot += player.BetAmount
	hand.Pot.MainPot.Players[player] = struct{}{}
	for _, p := range playersAscBet[i+1:] {
		p.BetAmount -= player.BetAmount
		hand.Pot.MainPot.Players[p] = struct{}{}
		hand.Pot.MainPot.Pot += player.BetAmount
	}
	hand.Pot.SidePots = append(hand.Pot.SidePots, hand.Pot.MainPot)
	hand.Pot.MainPot = SubPot{make(map[*Player]struct{}), 0}
}

func (hand *Hand) getPlayerRanking() [][]*Player {
	var pRank []*Player
	if hand.Players.Len() == 1 {
		return [][]*Player{{RingToPlayer(hand.Players)}}
	}
	hand.Players.Do(func(p interface{}) {
		player := p.(*Player)
		player.HandRank = poker.Evaluate(append(hand.Board, player.Hole...))
		pRank = append(pRank, player)
	})
	sort.Slice(pRank, func(p1 int, p2 int) bool {
		return pRank[p1].HandRank < pRank[p2].HandRank
	})
	playerRanking := [][]*Player{}
	playerRanking = append(playerRanking, []*Player{pRank[0]})
	rating := pRank[0].HandRank
	rank := 0
	for _, p := range pRank[1:] {
		if rating == p.HandRank {
			playerRanking[rank] = append(playerRanking[rank], p)
		} else {
			rank++
			playerRanking = append(playerRanking, []*Player{p})
		}
		rating = p.HandRank
	}
	return playerRanking
}

func (hand *Hand) distributePots(playerRanking [][]*Player) {
	for _, pot := range append(hand.Pot.SidePots, hand.Pot.MainPot) {
		for _, pRanking := range playerRanking {
			winners := []*Player{}
			for _, p := range pRanking {
				if _, ok := pot.Players[p]; ok {
					winners = append(winners, p)
				}
			}
			if len(winners) > 0 {
				minWinnings := pot.Pot / len(winners)
				for i, p := range winners {
					if i == len(winners)-1 {
						p.Funds += pot.Pot
					} else {
						p.Funds += minWinnings
						pot.Pot -= minWinnings
					}
				}
				break
			}
		}
	}
}
