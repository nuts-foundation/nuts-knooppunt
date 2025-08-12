# nuts-knooppunt

Implementation of the Nuts Knooppunt specifications

## Running

Using Docker:

```shell
docker build . --tag nutsfoundation/nuts-knooppunt:local
docker run -p 8080:8080 nutsfoundation/nuts-knooppunt:local
```

## Endpoints

- Landing page: [http://localhost:8080](http://localhost:8080)
- Health check endpoint: [http://localhost:8080/health](http://localhost:8080/health)

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

## Components

This section lists the components of the application, commonly used endpoints and configuration options.

### Nuts node
The embedded [Nuts node](https://github.com/nuts-foundation/nuts-node) can be configured through environment variables prefixed with `NUTS_`, or by using a configuration file called `config.nuts.yaml`.

Endpoints:
- Public status page: [http://localhost:8080/nuts/status](http://localhost:8080/nuts/status)
- Internal diagnostics page: [http://localhost:8081/nuts/status/diagnostics](http://localhost:8081/nuts/status/diagnostics)
