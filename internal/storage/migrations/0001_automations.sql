CREATE TABLE automations (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    enabled     INTEGER NOT NULL,
    updated_at  TEXT NOT NULL
);

-- Seed a couple of illustrative automations so a fresh install isn't blank.
-- These are ordinary rows: the user can disable or (once supported) delete
-- them like any other.
INSERT INTO automations (id, name, description, trigger, enabled, updated_at) VALUES
    ('water-plants',  'Water the plants', 'Remind me to water plants every Tuesday morning.', 'schedule: 0 9 * * 2', 1, '2026-01-01T00:00:00Z'),
    ('inbox-summary', 'Inbox summary',    'Summarize my inbox every morning.',                'schedule: 0 8 * * *', 0, '2026-01-01T00:00:00Z');
