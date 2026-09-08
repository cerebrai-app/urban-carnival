-- cerebrai's initial schema. Kept as one migration while the schema is still
-- pre-release: no tagged build has run it, so it may be edited freely. Once
-- it ships, treat it as immutable and add further changes in a new
-- NNNN_*.sql migration instead.
--
-- This file is pure DDL: the schema a real build creates is completely empty.
-- Illustrative dev-only rows live in ../seeds/0001_dev_data.sql, applied only
-- in a developer's checkout (see internal/storage/storage.go migrate).

CREATE TABLE automations (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    enabled     INTEGER NOT NULL,
    -- source is the automation writer's authored code (DESIGN.md §5.5);
    -- empty until a draft is written.
    source      TEXT NOT NULL DEFAULT '',
    updated_at  TEXT NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    -- model is the chat model ID this session's replies are generated with
    -- (see chat.DefaultModel / chat.ProviderFor); empty means none assigned.
    model      TEXT NOT NULL DEFAULT '',
    -- provider_session_id is the chat provider's own conversation handle for
    -- this session (e.g. the Claude Code CLI's session_id in dev builds).
    -- Captured from the first assistant reply and replayed on later turns so
    -- the provider keeps its own context. Empty until the first reply, and
    -- for providers with no such concept.
    provider_session_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE messages (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    content    TEXT NOT NULL,
    -- thoughts is the model's streamed reasoning for an assistant message
    -- (DESIGN.md §5.2), shown collapsed above the reply in the UI. Empty for
    -- user messages and for providers that don't surface reasoning.
    thoughts   TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX messages_session_id_created_at ON messages (session_id, created_at);
