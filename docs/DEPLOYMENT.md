# Deployment

This page describes how to deploy the Knooppunt.

The diagrams on this page were created using [Structurizr](https://structurizr.com/), files are generated using `generate.sh`.

## Overview

![structurizr-A1_SystemContext.png](images/structurizr-A1_SystemContext.png)

### Technology

The Knooppunt is provided as a Docker image: `docker pull ghcr.io/nuts-foundation/nuts-knooppunt:0.1.2`
(see the [repository](https://github.com/nuts-foundation/nuts-knooppunt/pkgs/container/nuts-knooppunt)) for the latest version.

Refer to the Nuts node documentation for details on how to set up and configure the embedded Nuts node.

The Knooppunt requires a FHIR server for the mCSD Directories, you can use HAPI FHIR server for this.

## Deployment variants

This chapter describes several supported deployment options. There is a base deployment (version "A"),
and two variants (versions "B" and "C") that are intended for vendors who want to build on existing systems.

### Deployment "A"
Embedded Nuts node, "new" mCSD Query and Administration directories in the form of a HAPI FHIR server.
The vendor uses either the embedded mCSD Admin (web-)Application or the mCSD Administration Directory FHIR API to manage the mCSD entries.

![structurizr-A2_ContainerDiagram.png](images/structurizr-A2_ContainerDiagram.png)

### Deployment "B"
A variant of version "A" that uses an mCSD Administration Directory that is not managed through the embedded mCSD Admin (web-)Application.
This is often a facade on an existing care organization/endpoint database or API.

Intended for: vendors that have an existing system to administer care organization/endpoint information.

The following diagram shows the services involved from the Knooppunt's perspective:

![structurizr-B2_ContainerDiagram.png](images/structurizr-B2_ContainerDiagram.png)

The following diagram shows the services involved from the XIS' perspective:

![structurizr-B2_XIS_ContainerDiagram.png](images/structurizr-B2_XIS_ContainerDiagram.png)

### Deployment "C"
A variant of version "A" that uses an existing Nuts node, instead of the embedded Nuts node.

The following diagram shows the services involved from the Knooppunt's perspective:

![structurizr-C2_ContainerDiagram.png](images/structurizr-C2_ContainerDiagram.png)

The following diagram shows the services involved from the XIS' perspective:

![structurizr-C2_XIS_ContainerDiagram.png](images/structurizr-C2_XIS_ContainerDiagram.png)

## Generic Functions

This chapter describes when/how to deploy specific generic functions of the Knooppunt.

### Addressing

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

### Localization

To enable the NVI-endpoint of the Knooppunt, you need to provide a base URL for the NVI service.

The NVI will be provided by the ministry of health (VWS). During preliminary testing, an example NVI is available on this URL:

```
https://knooppunt-test.nuts-services.nl/nvi
```