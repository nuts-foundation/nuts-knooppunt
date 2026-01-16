This package contains embedded OPA policy bundles for the Nuts Knooppunt PDP component.

It will eventually be moved to configuration, instead of being compiled into the binary.

To regenerate the bundles after making changes to the policies, run:

```bash
go generate ./...
```