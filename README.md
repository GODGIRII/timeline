# Timeline

A collaborative web application for planning and tracking tasks and activities
on shared timelines, accessible from multiple devices.

## Current stage

The React web app and Go backend support individual accounts, shared spaces,
admin-controlled editing, tasks and events with deadlines and priorities, and
live activity notifications backed by durable storage. The responsive interface
includes list and calendar views, search, filters, and membership management.

The first layer defines how users connect to a shared space, gain access, and
receive changes from other participants. A space is the access boundary for a
shared timeline.

- [Connection and access design](docs/network-layer.md)
- [Running the server and using the API](docs/api.md)
- [MVP item rules, endpoints, and notifications](docs/mvp.md)
- [Frontend setup, structure, and browser tests](docs/frontend.md)

## Run locally

Requires Go 1.24 or newer and Node.js 22.12+ (or a newer supported release).

```sh
npm ci --prefix web
npm run build --prefix web
go run ./cmd/server -dev -origin http://localhost:8080
```

The API listens on `127.0.0.1:8080` and stores data in `data/timeline.db`.
Open **http://localhost:8080/**, create an account, and create or join a space.
Go serves the built frontend and API from the same origin. `GET /healthz` checks
liveness. Deployments must use HTTPS, either with the TLS flags or a reverse proxy.
Rebuild after frontend changes, or use the Vite development workflow in the
[frontend guide](docs/frontend.md). Build files are generated and not committed.

```sh
go test -race ./...
go vet ./...
```

## Repository structure

```text
cmd/server/                Server entry point and configuration
internal/auth/             Password hashing and opaque credentials
internal/spaces/           Records and versioned document writes
internal/timeline/         Task/event validation, versions, and activity rules
internal/storage/          Atomic embedded database transactions
internal/transport/        HTTP API, WebSockets, and integration tests
web/src/                   React components, live state, and styles
web/tests/                 Playwright tests against the real Go API
docs/                      Design and API usage
```

This is a single-server MVP. The embedded database needs no separate service.
Items, activities, and retry records are stored separately. Account and
membership metadata still use the original state record, and live snapshots load
all active items, so this version is intended for small deployments.
