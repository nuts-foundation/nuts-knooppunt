# eOverdracht Receiver Derived Authorization Policy

This policy implements a reverse search authorization pattern for eOverdracht receivers.
Instead of checking consent directly, it authorizes access to FHIR resources by following a chain from Task → Composition → Resources.

