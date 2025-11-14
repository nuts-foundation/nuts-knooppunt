# nuts-knooppunt

Implementation of the Nuts Knooppunt specifications.

## Endpoints

- Health check endpoint: [http://localhost:8081/status](http://localhost:8081/status)
- mCSD Admin Application: [http://localhost:8080/mcsdadmin](http://localhost:8080/mcsdadmin)
- mCSD Update Client force update: [POST http://localhost:8081/mcsd/update](http://localhost:8081/mcsd/update)
- NVI FHIR gateway endpoints:
  - Registration endpoint: [POST http://localhost:8081/nvi/DocumentReference](http://localhost:8081/nvi/DocumentReference)
  - Search endpoint:
    - [POST http://localhost:8081/nvi/DocumentReference/_search](http://localhost:8081/nvi/DocumentReference/_search)
    - [GET http://localhost:8081/nvi/DocumentReference](http://localhost:8081/nvi/DocumentReference)

## Configuration

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for configuration options and instructions.

## Deployment

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for how to deploy the Knooppunt in your XIS/healthcare system.

## Integration

See [docs/INTEGRATION.md](docs/INTEGRATION.md) for how to integrate the Knooppunt in your local XIS/healthcare system.

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for an overview of the architecture and design of the Knooppunt (for Knooppunt developers).

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

For a complete overview of the deployment options, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

A docker compose config is provided to run a set of services that are useful for development:

- HAPI server, multi-tenancy enabled, using UUIDs, running on port 8080
- PostgreSQL database, for the HAPI server

Start the stack with:

```shell
docker compose up --build
```

