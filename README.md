# curlx

A keyboard-driven terminal UI for exploring and executing APIs — Postman for the terminal.

Import OpenAPI specs from local files or remote git repositories, manage authentication contexts, cache request bodies, and fire requests without leaving your terminal.

---

## Install

### Homebrew
```bash
brew tap kumararoosh/curlx
brew install curlx
```

### Build from source
```bash
git clone https://github.com/kumararoosh/curlx
cd curlx
make build        # outputs to bin/curlx
```

---

## Quickstart

```bash
curlx
```

On first launch the UI opens with an empty endpoint list. Press `l` to load an OpenAPI spec and start exploring.

---

## Key Bindings

### Global (command mode)
| Key | Action |
|-----|--------|
| `Tab` | Cycle panes (endpoints → request → response) |
| `l` | Load an OpenAPI spec (local path or git URL) |
| `a` | Open auth context switcher |
| `b` | Open request body cache |
| `s` | Save current body to cache |
| `m` | Toggle mouse (off = terminal text selection, on = scrolling) |
| `q` / `ctrl+c` | Quit |

### Endpoint list
| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate |
| `enter` | Expand/collapse spec or folder, load endpoint into request pane |
| `/` | Filter endpoints |
| `x` | Remove the selected spec |

### Request pane (command mode)
| Key | Action |
|-----|--------|
| `enter` | Enter typing mode on the URL field |
| `i` | Enter typing mode on the body field |
| `ctrl+r` | Send request |

### Request pane (typing mode)
| Key | Action |
|-----|--------|
| `Tab` | Toggle between URL and body fields |
| `esc` | Return to command mode |

### Auth switcher
| Key | Action |
|-----|--------|
| `enter` | Activate selected context (or `(no auth)` to clear) |
| `A` | Add a new auth context |
| `d` | Delete selected context |
| `q` / `esc` | Close |

### Body cache
| Key | Action |
|-----|--------|
| `enter` | Load selected body into the body field |
| `p` | Promote a scoped body to global |
| `d` | Delete selected body |
| `q` / `esc` | Close |

---

## Loading Specs

**Local file**
```
l  →  ./path/to/openapi.yaml
```

**Remote git repository**
```
l  →  https://github.com/org/repo[#branch] path/to/spec.yaml
```

Multiple specs can be loaded simultaneously. Each appears as a collapsible group in the endpoint list, with endpoints nested under path-prefix folders (e.g. all `/books/*` routes appear under a `/books` folder).

Spec sources are persisted and auto-reloaded on next launch.

---

## Authentication

curlx supports three auth types:

| Type | Header sent |
|------|-------------|
| `bearer` | `Authorization: Bearer <token>` |
| `apikey` | `<Header-Key>: <token>` (default `X-API-Key`) |
| `basic` | `Authorization: Basic ...` (token as `user:password`) |

Auth contexts are stored in an AES-256-GCM encrypted file in the XDG data directory (`~/.local/share/curlx/auth.enc` on Linux/macOS). The encryption key is derived from the machine hostname — no passwords required.

Select `(no auth)` at the top of the auth list to send requests without any auth header.

---

## Request Body Cache

Bodies are saved per endpoint (`operationID`) and can be promoted to global scope for reuse across endpoints. The cache is stored in a local SQLite database at `~/.local/share/curlx/cache.db`.

---

## Architecture

```
curlx/
├── cmd/curlx/          # Binary entrypoint
├── internal/
│   ├── tui/            # Bubble Tea UI
│   │   ├── app.go      # Root model, layout, key routing
│   │   ├── nav.go      # Nav tree (spec → folder → endpoint nodes)
│   │   └── messages.go # tea.Msg types, list items, commands
│   ├── spec/
│   │   ├── loader.go   # OpenAPI 3.x parsing via libopenapi
│   │   └── git.go      # Shallow-clone spec loading from git
│   ├── auth/
│   │   └── store.go    # AES-256-GCM encrypted auth context storage
│   ├── cache/
│   │   └── cache.go    # SQLite request body cache
│   ├── config/
│   │   └── config.go   # Persistent spec source list (XDG config dir)
│   └── http/
│       └── client.go   # HTTP client with auth injection
├── testserver/         # Books CRUD API on :9090 (OpenAPI spec included)
└── testserver2/        # Task Manager API on :9091 (OpenAPI spec included)
```

### How it works

**TUI** (`internal/tui`) — Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), a model-update-view framework. `App` is the root model. `appMode` controls whether a modal overlay (spec loader, auth switcher, body cache, new auth form) is active. Key routing branches on mode first, then on which pane has focus. Within the request pane, an `inputFocused` flag creates a typing sub-mode where all keystrokes go to the active text input rather than triggering shortcuts.

**Nav tree** (`internal/tui/nav.go`) — Endpoints from all loaded specs are assembled into a flat slice of `NavItem` nodes (`NavKindSpec` → `NavKindFolder` → `NavKindEndpoint`). A separate `visibleNavItems` pass filters this slice based on each node's `collapsed` state before handing it to the Bubble Tea list. Toggling a spec or folder node updates the slice in place and rebuilds the visible list.

**Spec loading** (`internal/spec`) — `libopenapi` parses the OpenAPI document into a typed model. Endpoints are extracted by walking all path items and HTTP methods. For git sources, `go-git` performs a shallow clone into a temp directory, reads the spec file, then cleans up.

**Auth storage** (`internal/auth`) — Contexts are serialised to JSON, encrypted with AES-256-GCM (random nonce prepended), and written to disk. The XDG data directory ensures platform-appropriate storage on macOS, Linux, and Windows.

**Body cache** (`internal/cache`) — SQLite via `modernc.org/sqlite` (pure Go, no CGO). Bodies are keyed by `(name, operation_id)` with a unique constraint; upsert semantics keep the latest version. Global bodies use `operation_id = ""`.

**HTTP client** (`internal/http`) — Thin wrapper around `net/http` that injects auth headers based on the active `auth.Context` type before executing the request.

---

## Tech Stack

| Layer | Library |
|-------|---------|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| TUI components | [Bubbles](https://github.com/charmbracelet/bubbles) (list, textinput, viewport) |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| OpenAPI parsing | [libopenapi](https://github.com/pb33f/libopenapi) |
| Git integration | [go-git](https://github.com/go-git/go-git) |
| SQLite | [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (CGO-free) |
| XDG directories | [adrg/xdg](https://github.com/adrg/xdg) |
| Distribution | [GoReleaser](https://goreleaser.com) + Homebrew |

---

## Test Servers

Two local test servers are included for development:

```bash
make server    # Books CRUD API   → http://localhost:9090
make server2   # Task Manager API → http://localhost:9091
```

Both use `admin` / `password` as credentials and return a bearer token from `POST /auth/login`. Load their specs with:

```
./testserver/openapi.yaml
./testserver2/openapi.yaml
```

---

## Distribution

Releases are automated via GitHub Actions. Pushing a `v*` tag triggers GoReleaser, which:

1. Builds binaries for macOS (arm64, amd64), Linux (arm64, amd64), and Windows (amd64)
2. Creates a GitHub release with checksums
3. Updates the [homebrew-curlx](https://github.com/kumararoosh/homebrew-curlx) tap formula automatically

```bash
git tag v1.0.0
git push origin v1.0.0
```
