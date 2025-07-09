# Generic Function Addressing

This module contains the code for the generic funtion addressing.

See the `/docs` for the design and usage documentation.

## Test setup

The `Directory` is really just a FHIR server. For testing there is docker compose file included which starts a multi tenant HAPI FHIR server which can be used as a directory.

To create the tenants, run the bruno scripts in the top level bruno directory. This create a `LRZa` and a `local` tenant.
