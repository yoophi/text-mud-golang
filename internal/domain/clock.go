package domain

import "time"

// Clock provides the current time to the game engine. Injecting a fake
// clock keeps time-based rules deterministic in tests.
type Clock interface {
	Now() time.Time
}

// Random provides bounded random numbers to the game engine. Injecting a
// fake random source keeps combat rolls reproducible in tests.
type Random interface {
	IntN(n int) int
}
