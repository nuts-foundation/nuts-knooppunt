# Monolith Architecture with Modular Subsystems

## Context and Problem Statement

The Nuts Knooppunt needs to integrate multiple subsystems including authentication (Nuts node), authorization (OPA), addressing (mCSD), localization, and consent management. Currently, these would require deploying and managing multiple containers, which adds operational complexity for parties deploying a Knooppunt. We need to decide on the overall architecture approach to favor simplicity and "plug and play" deployment while maintaining flexibility for future scaling and supporting diverse target environments where parties may already have existing infrastructure (e.g., an existing Nuts node or OPA instance they want to reuse).

## Considered Options

* **Multiple containers/microservices** - Each subsystem as a separate container requiring individual deployment and management
* **Pure monolith with function calls** - Single container with tightly integrated subsystems communicating via direct function calls for maximum efficiency
* **Jaeger-style all-in-one** - Single binary that can run either in all-in-one mode (all components) or with subcommands to run individual components separately (e.g., `jaeger-all-in-one` vs `jaeger-collector`, `jaeger-query`). This provides deployment flexibility while maintaining a single codebase

## Decision Outcome

Chosen option: "Hybrid monolith combining all-in-one deployment with HTTP communication", which combines elements from multiple approaches:

**From the Jaeger-style all-in-one approach:**
* Single binary deployment
* Component lifecycle management allowing selective enablement
* Potential for future subcommands to run components individually

**From the monolith with HTTP communication approach:**
* HTTP-based internal communication instead of function calls
* Clear API boundaries between subsystems
* Each subsystem mounts its handlers on shared HTTP servers

**Key benefits of this combined approach:**
* **Simplified deployment** - Default all-in-one mode for easy "plug and play" deployment
* **Deployment flexibility** - Components can be selectively enabled/disabled via configuration
* **Clear boundaries** - HTTP APIs prevent tight coupling and enable testing/mocking
* **Future-proof** - Can evolve to run components separately or extract to microservices
* **Technical feasibility** - All components available in Go with shared lifecycle interface

This hybrid approach gives us the simplicity of a monolith with the architectural flexibility of a more modular system.

## Implementation Details

### HTTP Interface Design

**Two separate HTTP servers:**
- **Public API** (port 8080) - External-facing endpoints
- **Private API** (port 8081) - Internal/health endpoints

**URL structure example:**
- `host:8080/{component}/*` - Public component endpoints (e.g., `/auth/*`, `/addressing/*`)
- `host:8081/health` - Health check endpoint
- `host:8081/admin/*` - Administrative interfaces
- `host:8081/nuts/internal/*` - Nuts node's internal endpoints

### Inter-subsystem Communication

**Deliberate choice: HTTP over in-process calls**

Despite being in the same process, subsystems communicate via HTTP APIs rather than direct function calls. This works as follows:
- Each component registers HTTP handlers on the shared servers (ports 8080/8081)
- When component A needs to call component B, it makes an HTTP request to `localhost:8080/component-b/*`
- The request goes through the network stack (even though it's localhost) and is handled by component B's registered handlers

This is an intentional architectural decision:
- **Loose coupling** - Components remain independent with clear API contracts
- **Future extraction** - Any component can be moved to a separate service without code changes
- **Testing isolation** - Components can be tested independently with HTTP mocks
- **Consistent interface** - Same API whether component is embedded or remote

**Trade-off acknowledged:** This adds HTTP overhead (latency, serialization) and more verbose tracing (each internal HTTP call creates additional spans) compared to direct function calls, but we accept this cost for the architectural benefits and future flexibility.

### Configuration Management

- **Component toggles** - Each component can be enabled/disabled via configuration

## Key Trade-offs

* **Performance vs Flexibility** - Accepting HTTP overhead for cleaner architecture and future extraction capability
* **Deployment simplicity vs Scaling granularity** - Single binary is easier to deploy but can't scale components independently
* **Version lock-in** - All embedded components must be compatible, can't mix versions like in microservices