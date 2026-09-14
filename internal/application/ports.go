// Package application coordinates the game engine with inbound (network)
// and outbound (persistence, output) adapters through ports.
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
