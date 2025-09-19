# Deployment

Nuts Knooppunt is provided as a single docker image that should be deployed within a participating organisation's
network. We will update this guide as we implement the different generic functions.

Knooppunt is available from the GitHub Container Registry.

https://github.com/nuts-foundation/nuts-knooppunt/pkgs/container/nuts-knooppunt

Knooppunt embeds a Nuts Node which is disabled by default.

## Addressing

To participate in the addressing generic function Knooppunt will connect to several FHIR APIs to synchronize data, according to the mCSD profile:
- a Root Administration Directory, authoratitive on care organizations and their Administration Directory endpoints,
- Administration Directories of care organizations, discovered through the Root Administration Directory.
- a local Query Directory where the synchronisation process will put data received from other organisations

For your own Knooppunt, you need to:
- provide a FHIR API for the Query Directory
- provide a FHIR API for the Administration Directory, containing your care organization registrations, that other Knooppunt instances can query.
  - You can use the embedded mCSD Admin Editor web application (`/mcsdadmin`) to maintain this directory.
- configure the Root Administration Directory

A multi tenant HAPI server can be used for hosting both the admin and query directory. We recommend to keep this data
separate, but you can choose to combine the data in a single tenant if so desired.

The root directory will be the LRZA directory provided by the ministry of health (VWS). During preliminary testing, an example root directory is available on this URL:

```
https://knooppunt-test.nuts-services.nl/lrza/mcsd
```

Please get in contact if you would like to make your Administration Directory discoverable through our example
root directory.

For testing purposes your admin directory should be reachable through the public internet. The production scenario aims
to utilise mTLS for trusted communication.

For full configuration options see our [Configuration Guide](./CONFIGURATION.md)

## Localization

To enable the NVI-endpoint of the Knooppunt, you need to provide a base URL for the NVI service.

The NVI will be provided by the ministry of health (VWS). During preliminary testing, an example NVI is available on this URL:

```
https://knooppunt-test.nuts-services.nl/nvi
```