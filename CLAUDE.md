# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A deliberately lightweight **decoy website** in Go + htmx. It imitates an ordinary corporate
site and exposes an `/admin` login form that **must never authenticate** — there is no real
backend, no database, no sessions. The whole app is a single static binary with templates, CSS
and htmx embedded via `//go:embed`.

## Commands

```sh
make build          # CGO_ENABLED=0 static binary → ./dummypage (injects revision via ldflags)
make run            # go run ./app --dbg
make test           # go test -race -count=1 ./...
go test -count=1 -run TestAdminLoginNeverAuthenticates ./app/server   # single test
go run ./app --live # run with the full site instead of the maintenance page
go run ./app --help # list all flags
```

There is no linter wired beyond `go vet` (the `make lint` golangci-lint step is best-effort).

## Architecture

- **`app/main.go`** — tiny `main()`: version print → `go-flags` parse of the global `opts`
  struct → `setupLog` (`go-pkgz/lgr`) → build `server.Server` → `Run(ctx)` with
  signal-based graceful shutdown. `revision` is set at build time via `-ldflags "-X main.revision=..."`.
- **`app/server/`** — the whole HTTP app.
  - `server.go`: the `Server` struct (config fields copied from `opts`), template parsing, the
    `routegroup` router, and the `//go:embed web/templates` + `//go:embed web/static` directives.
  - `handlers.go`: page handlers, the contact form, and `adminLogin`.
  - `web/templates/*.html`, `web/static/{style.css,htmx.min.js}`: embedded assets.

### Two cross-cutting behaviors to preserve

1. **`/admin` never authenticates.** `adminLogin` reads `username`/`password` *only* for
   optional logging, compares them against nothing, sleeps 1–2s to mimic a real check, and
   always renders the rejection. There must be no credential check, cookie, session, token, or
   redirect anywhere. `TestAdminLoginAlwaysFails` and `TestAdminLoginNeverAuthenticates` lock
   this in — do not weaken them.
2. **Maintenance mode** (default on). `maintenanceGate` middleware serves an "under
   development" page (HTTP 503) for every path **except** `/static/` (so CSS loads) and
   `/admin` (the login form stays live). `--live` / `LIVE=true` disables it and reveals the
   full site. The full-site code stays in the tree either way.

### Config toggles (flag or env)

`go-flags` bool flags cannot take `--flag=false`, so the two "on by default" concerns are
expressed as **negative** flags that default false: `--live`/`LIVE` (show full site) and
`--no-log`/`NO_LOG` (disable attempt logging). Others: `--host`/`LISTEN_HOST`,
`--port`/`LISTEN_PORT`, `--log-passwords`/`LOG_PASSWORDS`, `--trust-proxy`/`TRUST_PROXY`,
`--dbg`/`DEBUG`. When adding a default-true toggle, invert it the same way rather than relying
on a `default:"true"` tag.

`--tls-cert`/`TLS_CERT` + `--tls-key`/`TLS_KEY` enable native HTTPS: when both are set, `Run`
uses `ListenAndServeTLS` with `tls.Config{MinVersion: TLS1.2}` instead of `ListenAndServe`;
`main` rejects the pair unless both are provided. Empty = plain HTTP behind a proxy (the default).

## Conventions

- Keep it dependency-light and stdlib-first; current deps (`go-flags`, `go-pkgz/lgr`,
  `go-pkgz/routegroup`) are intentionally tiny. Don't pull in a web framework or an ORM.
- All links are **root-relative** (`/about`, `/static/…`), so the app must be mounted at the
  root of a domain — sub-path hosting would need proxy rewriting. See `deployments/` for
  nginx/Caddy/Traefik/Apache and systemd examples.
- Log login attempts via `lgr` and run user-supplied strings through `truncate()` before
  logging. Honor `TrustProxy` (`X-Real-IP` → first `X-Forwarded-For` hop) only when enabled.
- The `Server: nginx` header is set on purpose so the binary doesn't advertise Go.
