# Deployment

Nuts Knooppunt is provided as a single docker image that should be deployed within a participating organisation's
network. We will update this guide as we implement the different generic functions.

Knooppunt is available from the GitHub Container Registry.

https://github.com/nuts-foundation/nuts-knooppunt/pkgs/container/nuts-knooppunt

Knooppunt embeds a Nuts Node which is disabled by default.

## Addressing

To participate in the addressing generic function Knooppunt will need to connect to three different FHIR services
providing data according to the mCSD profile.

1. A root directory keeping authoritative references to a set of admin directories
2. An admin directory used to publish data such as endpoints
3. A query directory where the synchronisation process will place data received from other organisations

A multi tenant HAPI server can be used for hosting both the admin and query directory. We recommend to keep this data
separate, but you can choose to combine the data in a single tenant if so desired.

The root directory will be the LRZA directory provided by the ministry of health (VWS). During testing Nuts will provide
an example root directory which is available on this URL:

https://knooppunt-test.nuts-services.nl/lrza/mcsd

Please notify Nuts if you would like to make a directory discoverable through our example
root directory.

For testing purposes your admin directory should be reachable through the public internet. The production scenario aims
to utilise mTLS for trusted communication.

For full configuration options see our [Configuration Guide](./CONFIGURATION.md)
