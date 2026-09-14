package domain

// Effect is a result of a game state change that must be observed by the
// outside world (network output, persistence, shutdown). The game engine
// never performs I/O itself; it only returns effects for the application
// layer to apply.
type Effect interface{ isEffect() }

// Output sends Text to a single session.
type Output struct {
	Session SessionID
	Text    string
}

// Broadcast sends Text to every listed session.
type Broadcast struct {
	Sessions []SessionID
	Text     string
}

// SaveCharacter asks the application layer to persist a snapshot of the
// character taken at effect creation time.
type SaveCharacter struct {
	Character *Character
}

// Shutdown asks the application layer to stop the server gracefully.
type Shutdown struct {
	Reason string
}

func (Output) isEffect()        {}
func (Broadcast) isEffect()     {}
func (SaveCharacter) isEffect() {}
func (Shutdown) isEffect()      {}
