# Nuts Node Subsystem Integration

This document describes the implementation of the Nuts node as an embedded subsystem in the Knooppunt application.

## Architecture Overview

The implementation follows **Option 6** from the issue discussion: "Let Nuts node manage its own HTTP server interfaces (bound to 127.0.0.1:8080 and 127.0.0.1:8081), and create proxy inside of Nuts Knooppunt which routes to these "internal" HTTP interface."

### Components

1. **Subsystem Interface** (`internal/subsystems/subsystem.go`)
   - Defines contract for embeddable subsystems
   - Provides lifecycle management (Start/Stop)
   - Specifies routing information

2. **Subsystem Manager** (`internal/subsystems/manager.go`)
   - Manages registration and lifecycle of multiple subsystems
   - Creates proxy handlers for each subsystem
   - Handles graceful startup and shutdown

3. **Proxy Handler** (`internal/proxy/handler.go`)
   - Routes requests from main server to subsystem servers
   - Strips route prefixes when forwarding requests
   - Provides error handling and logging

4. **Nuts Subsystem** (`internal/subsystems/nuts/nuts.go`)
   - Implements the Nuts node as a subsystem
   - Manages separate public and internal HTTP servers
   - Currently a placeholder implementation

## Request Flow

1. Client sends request to `http://localhost:8080/nuts/api/endpoint`
2. Main Knooppunt server receives the request
3. Proxy handler matches the `/nuts/` prefix
4. Request is forwarded to `http://127.0.0.1:8280/api/endpoint`
5. Nuts node subsystem processes the request
6. Response is returned through the proxy to the client

## Port Allocation

- **Main Knooppunt**: `:8080` (public interface)
- **Nuts Public**: `127.0.0.1:8280` (internal, proxied via `/nuts/`)
- **Nuts Internal**: `127.0.0.1:8281` (internal, administrative interface)

## Benefits

1. **Framework Independence**: Nuts node can use Echo framework without conflicts
2. **Clean Separation**: Each subsystem manages its own HTTP server lifecycle
3. **Scalability**: Easy to add more subsystems with different route prefixes
4. **Maintainability**: Clear interfaces and separation of concerns
5. **Security**: Internal interfaces are bound to localhost only

## Future Enhancements

1. **Actual Nuts Node Integration**: Replace placeholder with real Nuts node dependency
2. **Configuration**: Add configuration for ports and subsystem settings
3. **Health Aggregation**: Combine health checks from all subsystems
4. **Metrics**: Add monitoring and metrics collection
5. **Load Balancing**: Support multiple instances of subsystems

## Testing

The implementation includes:
- Unit tests for subsystem integration
- End-to-end tests for request routing
- Health check validation
- Graceful shutdown testing

Run tests with:
```bash
go test ./...
```

## Example Usage

```go
// Create and register subsystem
manager := subsystems.NewManager()
nutsSubsystem := nuts.NewSubsystem()
manager.Register(nutsSubsystem)

// Start all subsystems
manager.Start(context.Background())

// Create main handler with subsystem routes
handler := manager.CreateHandler()
```