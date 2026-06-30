# yze-sql-keywordcase

A `yze-sql` family analyzer that reports PostgreSQL **keywords not written in lowercase** (uppercase is the error), per the gomatic SQL standard. It tokenizes each `.sql` file with the shared [`gomatic/go-sql`](https://github.com/gomatic/go-sql) library — a thin wrapper over `libpg_query`, PostgreSQL's own lexer — and flags any keyword token whose text is not already lowercase.

- **Rule:** `yze-sql/keywordcase` · **Tool:** `yze-sql`
- **Library:** `Diagnostics(path, source)` and `Report(read, files)` emit `go-yze` `Diagnostic`s (the stickler-json contract); a lexical scan error comes back wrapped in `go-sql`'s `ErrScan`.
- **Binary:** `cmd/yze-sql-keywordcase <paths...>` walks directories for `*.sql`, analyzes, and prints the stickler-json report — so [`stickler`](https://github.com/gomatic/stickler) runs it as a **declarative runner** (a `.stickler.yaml` `define:` entry, `format: stickler-json`).

Because `go-sql` wraps full PostgreSQL, the recognized keyword set is complete (not a DDL subset).
