package model

type (
	SignalType int

	TableSignal int

	Signal struct {
		PlayerName string
		SignalType SignalType
		Action     RoundAction
		Amount     int
		Message    string
	}
)

const (
	TableS   = SignalType(iota)
	BetS     = SignalType(iota)
	MessageS = SignalType(iota)

	SitS   = TableSignal(iota)
	StandS = TableSignal(iota)
)
