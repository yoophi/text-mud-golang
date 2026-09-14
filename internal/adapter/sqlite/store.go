// Package sqlite implements the application.CharacterRepository port on
// top of SQLite with versioned migrations.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yoophi/text-mud-golang/internal/application"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is a SQLite-backed character repository.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies every
// pending migration. Each migration runs in its own transaction.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1) // single writer; the game loop is sequential anyway
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite pragma: %w", err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// migrate applies embedded SQL files in version order exactly once.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		version, err := strconv.Atoi(strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql"), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("bad migration filename %s: %w", name, err)
		}
		var applied bool
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(new(int)); err == nil {
			applied = true
		} else if err != sql.ErrNoRows {
			return err
		}
		if applied {
			continue
		}
		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// storedItem mirrors the inventory JSON representation.
type storedItem struct {
	ID    string `json:"id"`
	Proto string `json:"proto"`
}

// Load returns the persisted character or application.ErrCharacterNotFound.
func (s *Store) Load(ctx context.Context, name string) (*domain.Character, error) {
	var (
		room      string
		inventory string
	)
	err := s.db.QueryRowContext(ctx, `SELECT room_id, inventory FROM characters WHERE name = ?`, name).
		Scan(&room, &inventory)
	if err == sql.ErrNoRows {
		return nil, application.ErrCharacterNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load character %s: %w", name, err)
	}
	c := &domain.Character{Name: name, Room: domain.RoomID(room), MaxHP: domain.DefaultMaxHP, HP: domain.DefaultMaxHP}
	var items []storedItem
	if err := json.Unmarshal([]byte(inventory), &items); err != nil {
		return nil, fmt.Errorf("decode inventory of %s: %w", name, err)
	}
	for _, it := range items {
		c.Inventory = append(c.Inventory, domain.ItemInstance{ID: it.ID, Proto: domain.ItemProtoID(it.Proto)})
	}
	return c, nil
}

// Save creates or updates the character atomically.
func (s *Store) Save(ctx context.Context, c *domain.Character) error {
	items := make([]storedItem, 0, len(c.Inventory))
	for _, it := range c.Inventory {
		items = append(items, storedItem{ID: it.ID, Proto: string(it.Proto)})
	}
	blob, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO characters (name, room_id, inventory, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET room_id = excluded.room_id, inventory = excluded.inventory, updated_at = excluded.updated_at`,
		c.Name, string(c.Room), string(blob), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save character %s: %w", c.Name, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("save character %s: affected %d rows", c.Name, n)
	}
	return tx.Commit()
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }
