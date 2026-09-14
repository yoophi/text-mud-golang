-- 0001: initial schema. Characters are identified by name; the room is
-- the last known location. Inventory is a JSON array of item instances.
CREATE TABLE characters (
    name       TEXT PRIMARY KEY,
    room_id    TEXT NOT NULL,
    inventory  TEXT NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
