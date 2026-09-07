# nuts-knooppunt

Implementation of the Nuts Knooppunt specifications.

[Documentation](https://nuts-foundation.github.io/nuts-knooppunt/)

## Demo EHR

A demonstration Electronic Health Record (EHR) application showcasing Dutch healthcare data exchange use cases including BGZ exchange and eOverdracht workflows.

See [mock-components/demo-ehr/README.md](mock-components/demo-ehr/README.md) for detailed documentation and setup instructions.

## Endpoints

- Health check endpoint: [http://localhost:8081/status](http://localhost:8081/status)
- mCSD Admin Application: [http://localhost:8080/mcsdadmin](http://localhost:8080/mcsdadmin)
- mCSD Update Client force update: [POST http://localhost:8081/mcsd/update](http://localhost:8081/mcsd/update)
- NVI FHIR gateway endpoints:
  - Registration endpoint: [POST http://localhost:8081/nvi](http://localhost:8081/nvi) (transaction Bundle) or [POST http://localhost:8081/nvi/List](http://localhost:8081/nvi/List) (single `List`)
  - Search endpoint: [GET http://localhost:8081/nvi/List](http://localhost:8081/nvi/List)
- Demo EHR Application: [http://localhost:3000](http://localhost:3000)

## Configuration

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for configuration options and instructions.

## Deployment

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for how to deploy the Knooppunt in your XIS/healthcare system.

## Integration

See [docs/INTEGRATION.md](docs/INTEGRATION.md) for how to integrate the Knooppunt in your local XIS/healthcare system.

## API Reference

Browse it at [https://nuts-foundation.github.io/nuts-knooppunt/docs/api/](https://nuts-foundation.github.io/nuts-knooppunt/docs/api/).

- [openapi.yaml](openapi.yaml): status, PDP, NVI and MITZ subscription APIs (internal interface).
- [mitz-public.openapi.yaml](mitz-public.openapi.yaml): the single public-interface endpoint (`/mitz/notify`), called by MITZ directly.

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

- **Knooppunt** - Main application running on ports 8080 (API) and 8081 (internal)
- **HAPI FHIR Server** - Multi-tenant FHIR R4 server with NVI support, running on port 7050
- **Aspire Dashboard** - Observability dashboard for traces, logs, and metrics on port 18888
- **Demo EHR** - Demo application (optional, use `--profile demoehr`)
- **PEP (Policy Enforcement Point)** - NGINX-based reference implementation, one per demo organization: Zonnebloem on port 9080 and De Plataan on port 9081

Start the base stack with:

```shell
docker compose up --build
```

Start with demo-ehr:

```shell
docker compose --profile demoehr up --build
```

