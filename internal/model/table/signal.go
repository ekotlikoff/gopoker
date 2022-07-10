package model

type (
	// SignalType TODO is this needed for anything?
	SignalType int

	// TableSignal TODO is this needed for anything?
	TableSignal int

	// Signal TODO is this needed for anything?
	Signal struct {
		PlayerName string
		SignalType SignalType
		Action     RoundAction
		Amount     int
		Message    string
	}
)

const (
	// TableS TODO is this needed for anything?
	TableS = SignalType(iota)
	// BetS TODO is this needed for anything?
	BetS = SignalType(iota)
	// MessageS TODO is this needed for anything?
	MessageS = SignalType(iota)

	// SitS TODO is this needed for anything?
	SitS = TableSignal(iota)
	// StandS TODO is this needed for anything?
	StandS = TableSignal(iota)
)
