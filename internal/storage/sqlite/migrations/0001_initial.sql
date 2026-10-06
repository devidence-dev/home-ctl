-- Migration 0001: Initial schema for home-ctl

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    username TEXT,
    locale TEXT NOT NULL DEFAULT 'en',
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS user_onboarding (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    connected INTEGER NOT NULL DEFAULT 0 CHECK (connected IN (0, 1)),
    connected_at TEXT,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS shopping_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    emoji TEXT NOT NULL,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS shopping_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id INTEGER NOT NULL REFERENCES shopping_items(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'purchased')),
    added_by INTEGER NOT NULL REFERENCES users(id),
    added_at TEXT NOT NULL,
    purchased_by INTEGER REFERENCES users(id),
    purchased_at TEXT
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_shopping_entries_pending 
ON shopping_entries(item_id) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS chore_definitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    emoji TEXT NOT NULL,
    description TEXT,
    recurrence_interval INTEGER CHECK (recurrence_interval IS NULL OR recurrence_interval > 0),
    recurrence_unit TEXT CHECK (recurrence_unit IS NULL OR recurrence_unit IN ('day', 'week', 'month')),
    recurrence_anchor TEXT CHECK (recurrence_anchor IS NULL OR recurrence_anchor IN ('scheduled', 'completion')),
    series_origin TEXT,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS chore_occurrences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    definition_id INTEGER NOT NULL REFERENCES chore_definitions(id),
    assignee_id INTEGER NOT NULL REFERENCES users(id),
    scheduled_at TEXT NOT NULL,
    due_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open', 'completed')),
    completed_by INTEGER REFERENCES users(id),
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (definition_id, scheduled_at)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_chore_occurrences_assignee_status_due
ON chore_occurrences(assignee_id, status, due_at);

CREATE TABLE IF NOT EXISTS reminder_definitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    emoji TEXT,
    description TEXT,
    target_user_id INTEGER NOT NULL REFERENCES users(id),
    recurrence_interval INTEGER CHECK (recurrence_interval IS NULL OR recurrence_interval > 0),
    recurrence_unit TEXT CHECK (recurrence_unit IS NULL OR recurrence_unit IN ('day', 'week', 'month')),
    recurrence_anchor TEXT CHECK (recurrence_anchor IS NULL OR recurrence_anchor IN ('scheduled', 'completion')),
    series_origin TEXT,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS reminder_occurrences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    definition_id INTEGER NOT NULL REFERENCES reminder_definitions(id),
    target_user_id INTEGER NOT NULL REFERENCES users(id),
    scheduled_at TEXT NOT NULL,
    due_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open', 'completed')),
    completed_by INTEGER REFERENCES users(id),
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (definition_id, scheduled_at)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_reminder_occurrences_target_status_due
ON reminder_occurrences(target_user_id, status, due_at);

CREATE TABLE IF NOT EXISTS postponements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('chore', 'reminder')),
    occurrence_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id),
    old_due_at TEXT NOT NULL,
    new_due_at TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS notification_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('chore', 'reminder')),
    occurrence_id INTEGER NOT NULL,
    target_user_id INTEGER NOT NULL REFERENCES users(id),
    due_at TEXT NOT NULL,
    attempt_time TEXT NOT NULL,
    attempt_seq INTEGER NOT NULL DEFAULT 1,
    outcome TEXT NOT NULL CHECK (outcome IN ('claimed', 'sent', 'failed')),
    failure_reason TEXT,
    telegram_message_id INTEGER,
    created_at TEXT NOT NULL,
    UNIQUE (entity_type, occurrence_id, due_at, attempt_seq)
) STRICT;

CREATE TABLE IF NOT EXISTS flow_state (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    chat_id INTEGER NOT NULL,
    thread_id INTEGER NOT NULL DEFAULT 0,
    step TEXT NOT NULL,
    payload TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (user_id, chat_id, thread_id)
) STRICT;

CREATE TABLE IF NOT EXISTS processed_updates (
    update_id INTEGER PRIMARY KEY,
    processed_at TEXT NOT NULL
) STRICT;
