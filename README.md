# yze-sql-keywordcase

A SQL analyzer in the [`yze`](https://github.com/gomatic/yze) suite that reports PostgreSQL **keywords not written in lowercase** (uppercase is the error), per the gomatic SQL standard. It tokenizes each `.sql` file with the shared [`gomatic/go-sql`](https://github.com/gomatic/go-sql) library — a thin wrapper over `libpg_query`, PostgreSQL's own lexer — and flags any keyword token whose text is not already lowercase.

- **Rule:** `yze/keywordcase` · **Tool:** `yze` · **Category:** `sql` (the `yze-sql-*` repo name carries the language group; the rule id stays flat).
- **Library:** `Diagnostics(path, source)` and `Report(read, files)` emit `go-yze` `Diagnostic`s (the stickler-json contract); a lexical scan error comes back wrapped in `go-sql`'s `ErrScan`.
- **Bundled into `yze`:** the [`yze`](https://github.com/gomatic/yze) aggregator imports this analyzer and runs it over the `.sql` files under its pattern roots, so `yze` (and `stickler`, which runs `yze`) lint SQL alongside Go. The standalone `cmd/yze-sql-keywordcase <paths...>` binary remains for direct use.

Because `go-sql` wraps full PostgreSQL, the recognized keyword set is complete (not a DDL subset).
