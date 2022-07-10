A `Table` consists of `Players` that may be sitting (and playing) or standing (and watching). Play consists of a series of `Hand`s.

Each `Hand` consists of the dealing of cards to each player, the dealing of shared cards in the `Board`, and the orchestration of betting in the form of `Round`s.

Once only one `Player` remains playing in the `Hand` or the final bets have been made, winners are identified (usually one winner, but there can be multiple in the case of an all in and split pot or ties). The winners are granted their winnings and the next Hand is dealt.
