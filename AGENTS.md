# AGENTS.md — Incipit

This repo is **spec-only**: `SPEC.md` is the authoritative source of truth and no code exists yet. Read `SPEC.md` in full before implementing anything. This file only captures high-signal constraints that are easy to get wrong or guess at.

## Toolchain

- Go 1.22, module path `github.com/jason/incipit`
- **Pure-Go SQLite** via `modernc.org/sqlite` — no CGO. Build with `CGO_ENABLED=0`. Do not switch to `mattn/go-sqlite3` (CGO breaks the scratch image / cross-compile goal).
- Dependency allowlist: `modernc.org/sqlite`, `github.com/go-chi/chi/v5`, `github.com/go-chi/cors`, plus Go stdlib only. Do not introduce other third-party deps without checking the spec.
- HTTP router: chi. Web UI: server-rendered `html/template` — no SPA, no JS framework, no CSS framework.

## Layout

All app code lives under `internal/`:

```
internal/{db,epub,lookup,models,opds,server,storage}/
web/{templates,static}/
main.go                 # subcommand dispatch
```

See SPEC.md "Project Structure" for the full tree.

## CLI subcommands

`init` · `serve` · `parse <path>` · `lookup [--isbn X | --title T --author A]` · `add <path> [--no-lookup] [--dry-run]` · `add-user --username --password [--role admin]` (also resets password if user exists) · `list-users` · `remove-user --username` · `import-calibre <dir>`

## Config (env vars)

- `INCIPIT_DB_PATH` — SQLite file path (default `/data/books.db`)
- `INCIPIT_PORT` — HTTP port (default 8080)
- `INCIPIT_STORAGE_DIR` — root for `files/` and `covers/` (default `/data`)

On-disk layout under storage dir: `books.db`, `files/{id}.epub`, `covers/{id}.jpg`.

## Auth model — get this right

**Every endpoint requires basic auth EXCEPT `/health` and `/syncs/healthcheck`.** No toggle, no conditional logic. Both KOReader's OPDS browser and sync plugin support basic auth natively.

## Password hashing — easy to get wrong

KOReader MD5-hashes the password client-side before sending. The server stores **bcrypt(MD5(password))**. The CLI `add-user` command takes plaintext, MD5-hashes it to match what KOReader sends, then bcrypt-hashes for storage. Do not store the raw password or skip the MD5 step.

## External API lookups

- Open Library (primary, no auth): ISBN lookup + title/author search. Extract series from subjects matching `series:{name}` (strip the `series:` prefix). Set `User-Agent: incipit/0.1 (...)` for better rate limits.
- Google Books (fallback): ratings + descriptions. Does NOT provide series — use Open Library for series.
- Merge precedence: OL wins series/subjects/cover; GB wins rating/description/published date; first non-empty wins for title/author/pages/publisher.
- All responses cached in the `metadata_cache` table.

## KOReader progress sync

`document_hash` = MD5 of EPUB content, stored in `books.file_hash`. Progress keyed by `(book_id, user_id)`, latest writer wins. The `device` field is informational only. No `POST /syncs/register` endpoint — users are created via CLI only.

## Deployment

- Multi-stage Docker build → `FROM scratch` image containing only the binary and `web/`. Build with `go build -ldflags="-s -w"` and `CGO_ENABLED=0`.
- Target: k3s cluster via the existing `veridian-apps` Helm chart. Ingress host `incipit.veridiandynamics` (Traefik).
- Single PVC at `/data` holds the entire system state (DB + files + covers). Backup = copy that directory.
- Readiness/liveness probes hit `GET /health` (no auth).