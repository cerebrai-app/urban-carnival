# Project instructions

## CodeGraph

After writing or editing any code, run `codegraph sync` to update the index before continuing.

## Database schema

cerebrai is pre-release, so the schema is not versioned incrementally. There
are two schema files, both edited in place — never add a new `000N_*.sql`
migration:

- [internal/storage/migrations/0001_initial_schema.sql](internal/storage/migrations/0001_initial_schema.sql)
  — pure DDL. This is the schema a real build creates: completely empty.
- [internal/storage/seeds/0001_dev_data.sql](internal/storage/seeds/0001_dev_data.sql)
  — illustrative rows applied only in a developer's checkout
  (`devmode.Enabled()`).

When you make a schema change, edit the DDL in `0001_initial_schema.sql` (and
`0001_dev_data.sql` if the seed rows need to match), then delete the local
database so it is recreated from scratch on next launch. In dev mode the
database is `cerebrai.db` at the repo root.

