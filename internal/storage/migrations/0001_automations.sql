CREATE TABLE automations (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    enabled     INTEGER NOT NULL,
    updated_at  TEXT NOT NULL
);
