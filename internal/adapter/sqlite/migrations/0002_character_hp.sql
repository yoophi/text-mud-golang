-- 0002: combat vitals. HP is persisted so reconnects and restarts keep
-- combat state consistent (#14).
ALTER TABLE characters ADD COLUMN hp INTEGER NOT NULL DEFAULT 0;
