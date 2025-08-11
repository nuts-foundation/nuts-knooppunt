package subsystems

import "context"

// Subsystem represents a subsystem that can be embedded in the Knooppunt application
type Subsystem interface {
	// Name returns the name of the subsystem
	Name() string
	
	// Start starts the subsystem
	Start(ctx context.Context) error
	
	// Stop stops the subsystem
	Stop(ctx context.Context) error
	
	// PublicAddress returns the address where the public interface is listening
	PublicAddress() string
	
	// InternalAddress returns the address where the internal interface is listening (if any)
	InternalAddress() string
	
	// RoutePrefix returns the prefix under which this subsystem should be mounted
	RoutePrefix() string
}