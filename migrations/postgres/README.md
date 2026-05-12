# PostgreSQL Migrations

This directory uses `golang-migrate` naming conventions:

- `NNNNNN_name.up.sql`
- `NNNNNN_name.down.sql`

The schema currently lives in a single bootstrap file (`000001_init.up.sql` / `000001_init.down.sql`).
Add a new `000002_*.sql` pair when you need to evolve the schema after release.

Run migrations via repository script:

```bash
bash scripts/migrate.sh up
bash scripts/migrate.sh down 1
bash scripts/migrate.sh goto 1
bash scripts/migrate.sh version
```
