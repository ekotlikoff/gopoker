//go:build wasm && js && webclient

package main

import (
	"time"

	"github.com/chehsunliu/poker"
	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

// Various types of updates that can be sent to a client.
const (
	// NewHandUpdateT is the newest hand, and the player's hand.
	NewHandUpdateT = PlayerUpdateType(iota)
	// RoundUpdateT is the most recent RoundAction made by an opponent.
	RoundUpdateT
	// BetUpdateT is a notification to the current better that the table is awaiting their bet.
	BetUpdateT
	// DealUpdateT is the new set of community cards.
	DealUpdateT
	// HandOverUpdateT is the result of the latest hand.
	HandOverUpdateT
	// FullUpdateT is the full update including the entire table's state.
	FullUpdateT
	// TableUpdateT is a table action e.g. a pause, somewhat standing up, etc.
	TableUpdateT
	// StateUpdateT is an update to table state - table no longer playing, etc.
	StateUpdateT
)

type (
	// TableServerConfig defines the TableServer's opinion of how a given Table should be run.
	TableConfig struct {
		TimeToBet        time.Duration
		TimeBetweenHands time.Duration
		ModelConfig      model.TableConfig
	}
	// StateUpdate is an update to the table's state, for things that don't happen immediately or based
	// on a clear user action.
	// For example: folks looking to stand start standing after a hand, the table stops playing due to
	// too few players
	StateUpdate struct {
		PlayStopped bool
		NowStanding []string
	}
	// PlayerUpdateType is the type of table update sent to a client.
	PlayerUpdateType int
	// PlayerUpdate includes the various kinds of updates sent to a client.
	PlayerUpdate struct {
		// Type is the type of the update.
		Type PlayerUpdateType
		// Hole is the player's hole in the newest hand.
		Hole []poker.Card
		// BigBlind is the big blind for the current hand.
		BigBlind int
		// Dealer is the new hand's dealer.
		Dealer string
		// Board is the new set of community cards corresponding to a deal update.
		Board []poker.Card
		// Winners is the result of the latest hand.
		Winners []model.Winner
		// RoundAction is the round action for the current better.
		RoundAction model.RoundAction
		// CurrentBetter is the player that made the round action.
		CurrentBetter string
		// Table is the full model.Table.
		Table model.Table
		// TableAction is a table action that may be relevant to a client.
		TableAction TableAction
		// StateUpdate is an update to the table's state
		StateUpdate StateUpdate
	}

	// TableActionType is the type of action a player can take on a table outside of an ongoing game.
	TableActionType int

	// TableAction is a player's request to the table outside the scope of a given round.
	TableAction struct {
		TableActionType TableActionType
		TableName       string
		Seat            int
		TableConfig     TableConfig
	}

	// Credentials for authentication
	Credentials struct {
		Username string
	}

	// CurrentMatch serializable struct to bring client up to speed
	CurrentTable struct {
		Name        string
		TableConfig model.TableConfig
		Players     [model.MaxTableSize]*model.Player
		DealerIndex int
		Standers    map[string]*model.Player
		Hand        *model.Hand
	}

	// SessionResponse serializable struct to send client's session
	SessionResponse struct {
		Credentials Credentials
		AtTable     bool
		Table       CurrentTable
	}
)
