# nuts-knooppunt

Implementation of the Nuts Knooppunt specifications

## Running

Using Docker:

```shell
docker build . --tag nutsfoundation/nuts-knooppunt:local
docker run -p 8080:8080 nutsfoundation/nuts-knooppunt:local
```

## Endpoints

### Knooppunt Endpoints
- Landing page: [http://localhost:8080](http://localhost:8080)
- Health check endpoint: [http://localhost:8080/health](http://localhost:8080/health)

### Embedded Subsystems
- Nuts node: [http://localhost:8080/nuts/](http://localhost:8080/nuts/)
- Nuts node health: [http://localhost:8080/nuts/health](http://localhost:8080/nuts/health)

## Architecture

The Knooppunt application embeds subsystems using a proxy architecture:

1. **Subsystems** run their own HTTP servers on localhost ports
2. **Proxy handlers** route traffic from the main Knooppunt server to subsystem servers
3. **Route prefixes** separate different subsystems (e.g., `/nuts/` for the Nuts node)

This approach allows subsystems like the Nuts node (which uses Echo framework) to maintain their own HTTP server management while being integrated into the Knooppunt application.

### Subsystem Ports
- Nuts node public interface: `127.0.0.1:8280`
- Nuts node internal interface: `127.0.0.1:8281`

## Go toolchain

It's a typical Go application, so:

```shell
go test ./...
```

and:

```shell
go build .
./nuts-knoopppunt
```

## Development stack

A docker compose config is provided to run a set of services that are useful for development:

- HAPI server, multi-tenancy enabled, using UUIDs, running on port 8080
- PostgreSQL database, for the HAPI server

Start the stack with:

```shell
docker compose -f docker-compose.dev.yml up
```

