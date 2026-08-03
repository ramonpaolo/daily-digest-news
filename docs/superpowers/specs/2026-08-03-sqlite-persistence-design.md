# SQLite persistence design

## Decisions

- The learning archive uses a pure-Go SQLite driver (`modernc.org/sqlite`) so the static Distroless image remains unchanged.
- The default database path is `/data/daily-digest-news.sqlite3`; Zenifra must persist the entire `/data` directory.
- Only learning data is persisted in this iteration: configured interests, request snapshots, complete generated lessons, and delivery status.
- News stories and news summaries remain transient.
- `LEARNING_RETENTION_DAYS` defaults to 365; `0` disables automatic cleanup.
- Scheduled `morning` and `evening` slots are deduplicated by date across restarts. `startup` always creates a new lesson.
- The last eight sent lessons contribute metadata only to the next prompt; previous lesson text is not sent back to the LLM.

## Lifecycle

Each learning run creates a `generating` record, stores the complete lesson as `email_pending` before SMTP, and then marks it `sent` or `failed`. SQLite open/migration errors stop startup to prevent silent ephemeral execution. Runtime learning failures remain independent from the news runner.

The project runs with one active replica. The volume must be writable by `nonroot` and preserve SQLite WAL/SHM files. No HTTP history endpoint or public command is added.
