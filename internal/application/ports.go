package application

import (
	"context"
	"errors"

	"github.com/yoophi/text-mud-golang/internal/domain"
)

// ErrCharacterNotFound is returned by CharacterRepository.Load when no
// character with the given name exists.
var ErrCharacterNotFound = errors.New("캐릭터를 찾을 수 없습니다")

// CharacterRepository persists characters. The application layer depends
// only on this port; SQLite details stay behind it.
type CharacterRepository interface {
	// Load returns the persisted character, or ErrCharacterNotFound.
	Load(ctx context.Context, name string) (*domain.Character, error)
	// Save creates or updates the character atomically.
	Save(ctx context.Context, c *domain.Character) error
	Close() error
}

// InputKind describes what happened on a connection.
type InputKind int

const (
	InputConnected InputKind = iota
	InputLine
	InputDisconnected
)

// Input is one inbound event delivered by the network adapter to the
// single game loop.
type Input struct {
	Session domain.SessionID
	Kind    InputKind
	Line    string
}

// NetGateway is the outbound network port. Implementations must never
// block the caller for long: slow clients are dropped.
type NetGateway interface {
	// Listen starts accepting connections and returns the bound address.
	Listen(addr string) (string, error)
	Write(session domain.SessionID, text string)
	Close(session domain.SessionID)
	Stop()
}
