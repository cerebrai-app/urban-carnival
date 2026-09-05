-- Dev-only seed data. Applied by internal/storage/storage.go migrate only
-- when devmode.Enabled() (a developer's checkout); a real build never runs
-- this file, so its database stays completely empty until the user creates
-- something. Kept editable alongside migrations/0001_initial_schema.sql while
-- the schema is pre-release.

-- A couple of illustrative automations so a dev's fresh install isn't blank.
-- These are ordinary rows: the user can disable or (once supported) delete
-- them like any other.
INSERT INTO automations (id, name, description, trigger, enabled, updated_at) VALUES
    ('water-plants',  'Water the plants', 'Remind me to water plants every Tuesday morning.', 'schedule: 0 9 * * 2', 1, '2026-01-01T00:00:00Z'),
    ('inbox-summary', 'Inbox summary',    'Summarize my inbox every morning.',                'schedule: 0 8 * * *', 0, '2026-01-01T00:00:00Z');
