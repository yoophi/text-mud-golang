package application

import (
	"math/rand/v2"
	"time"
)

// SystemClock is the real-time Clock implementation.
type SystemClock struct{}

// Now returns the wall clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// SystemRandom is the real randomness source.
type SystemRandom struct{}

// IntN returns a random number in [0,n).
func (SystemRandom) IntN(n int) int { return rand.IntN(n) }
