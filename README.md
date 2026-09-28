# Timeline

A collaborative web application for planning and tracking tasks and activities
on shared timelines, accessible from multiple devices.

## Current stage

The first Go backend layer supports individual accounts, device sessions,
shared spaces, owner-approved membership, and authenticated WebSocket updates
backed by durable storage.

The first layer defines how users connect to a shared space, gain access, and
receive changes from other participants. A space is the access boundary for a
shared timeline.

- [Connection and access design](docs/network-layer.md)
- [Running the server and using the API](docs/api.md)

## Run locally

Requires Go 1.24 or newer.

```sh
go run ./cmd/server -dev -origin http://localhost:8080
```

The API listens on `127.0.0.1:8080` and stores data in `data/timeline.db`.
Open `http://localhost:8080/` for the backend landing page. `GET /healthz` checks
liveness. The timeline web interface is not built yet. Deployments
must use HTTPS, either with the TLS flags or a reverse proxy.

```sh
go test -race ./...
go vet ./...
```

## Repository structure

```text
cmd/server/                Server entry point and configuration
internal/auth/             Password hashing and opaque credentials
internal/spaces/           Records and versioned document writes
internal/storage/          Atomic embedded database transactions
internal/transport/        HTTP API, WebSockets, and integration tests
docs/                      Design and API usage
```

This is a single-server foundation. The timeline payload is currently a JSON
object for exercising synchronization; task rules and the web UI come next.
The embedded database needs no separate service. Storage serializes the whole
state per transaction and retains document events, so it is intended for small
deployments while the domain model is developed.
