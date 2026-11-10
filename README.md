# dummypage

A lightweight decoy site in Go + htmx. It mimics an ordinary corporate website and serves an
`/admin` login form that **never** authenticates. No database, sessions, or state — a single
static binary with embedded templates, CSS, and htmx. Uses around 8–12 MB RSS.

By default the server shows an **"under development"** page for every request. The full site
(home/about/services/contacts + `/admin`) is kept in the code and revealed with `--live`.

Structure: `app/` layout, configuration via `go-flags`, logging via `go-pkgz/lgr`,
routing via `go-pkgz/routegroup`, `revision` injected through ldflags.

## Features

- "Under development" page shown for every request by default (HTTP 503); the full site is
  one flag away (`--live`).
- Corporate site: `/`, `/about`, `/services`, `/contacts` with htmx navigation (`hx-boost`).
- `/contacts` — a request form accepted over htmx (nothing is stored).
- `/admin` — a login form that always returns "invalid username or password" after a short
  delay (imitating a real check). No cookies or sessions are ever issued.
- Optional logging of login attempts (IP, User-Agent, username, and optionally the password).
- `Server: nginx` header so the binary does not reveal that it is written in Go.

## Build and run

```sh
make build          # → ./dummypage
./dummypage         # "under development" page on 0.0.0.0:8080
./dummypage --live  # the full site instead
make run            # go run with debug logging
make test
```

## Configuration (flag or environment variable)

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--host` | `LISTEN_HOST` | `0.0.0.0` | Host/IP to listen on (e.g. `127.0.0.1` behind nginx) |
| `--port` | `LISTEN_PORT` | `8080` | Port to listen on |
| `--live` | `LIVE` | `false` | Serve the full site instead of the under-development page |
| `--no-log` | `NO_LOG` | `false` | Disable logging of `/admin` login attempts |
| `--log-passwords` | `LOG_PASSWORDS` | `false` | Write the submitted password to the log (only if logging is on) |
| `--trust-proxy` | `TRUST_PROXY` | `false` | Take client IP from `X-Real-IP` / `X-Forwarded-For` |
| `--tls-cert` | `TLS_CERT` | — | Path to TLS certificate (PEM); enables HTTPS together with `--tls-key` |
| `--tls-key` | `TLS_KEY` | — | Path to TLS private key (PEM) |
| `--dbg` | `DEBUG` | `false` | Debug logging |

Login attempts are logged by default; `--no-log` (or `NO_LOG=true`) turns that off.

`--tls-cert` and `--tls-key` must be given together; when both are set the server speaks HTTPS
directly (TLS 1.2+, HTTP/2 enabled) instead of plain HTTP. Without them it serves HTTP and is
meant to sit behind a TLS-terminating proxy (see below).

Examples:

```sh
# the under-development page (default)
./dummypage

# reveal the full corporate site + /admin
./dummypage --live        # or: LIVE=true ./dummypage

# full site, behind a reverse proxy, listen locally only
LISTEN_HOST=127.0.0.1 TRUST_PROXY=true ./dummypage --live

# full site but without logging login attempts
./dummypage --live --no-log

# log passwords too (for brute-force analysis)
./dummypage --live --log-passwords

# serve HTTPS directly with your own certificate (no reverse proxy)
./dummypage --live --port 443 --tls-cert /path/cert.pem --tls-key /path/key.pem
```

### Native HTTPS

Point `--tls-cert` / `--tls-key` at a PEM certificate and private key and the binary terminates
TLS itself — no proxy required. The cert can come from Let's Encrypt (`certbot`), your own CA, or
a self-signed pair for testing:

```sh
# self-signed cert for localhost (testing only)
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
  -keyout key.pem -out cert.pem -days 365 -subj "/CN=localhost" \
  -addext "subjectAltName=IP:127.0.0.1"

./dummypage --live --port 8443 --tls-cert cert.pem --tls-key key.pem
# → https://127.0.0.1:8443/
```

For a public domain, prefer a reverse proxy (below) that renews certificates automatically, or
feed the binary the proxy-managed cert/key paths.

## Docker

```sh
docker build --build-arg REVISION=$(git rev-parse --short HEAD) -t dummypage .
docker run --rm -p 8080:8080 dummypage
```

The image is built on `scratch` with `GOMEMLIMIT=16MiB` — minimal size and memory footprint.

## Behind a reverse proxy

The app is designed to run behind nginx / Caddy / Traefik / Apache: bind it to localhost and
let the proxy terminate TLS. Set `--trust-proxy` (`TRUST_PROXY=true`) so login-attempt logs
record the real client IP from `X-Real-IP` / `X-Forwarded-For`.

```sh
LISTEN_HOST=127.0.0.1 TRUST_PROXY=true ./dummypage --live
```

Ready-to-use examples live in [`deployments/`](deployments/):

| File | Setup |
|------|-------|
| `nginx.conf` | nginx reverse proxy (HTTP + TLS variant) |
| `Caddyfile` | Caddy with automatic HTTPS |
| `apache.conf` | Apache `mod_proxy` |
| `docker-compose.caddy.yml` | app + Caddy in Docker |
| `docker-compose.traefik.yml` | app + Traefik (Let's Encrypt) in Docker |
| `dummypage.service` | systemd unit (hardened) for the bare binary |

Note: mount the app at the **root** of a domain/subdomain. Links are root-relative
(`/static/…`, `/admin`), so serving it under a sub-path (e.g. `example.com/site/`) would need
path rewriting on the proxy.
