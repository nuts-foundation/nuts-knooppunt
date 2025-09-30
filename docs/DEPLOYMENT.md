# Documentation

The diagrams on this page were created using [Structurizr](https://structurizr.com/), files are generated using `generate.sh`.

## Overview

![structurizr-A1_SystemContext.png](images/structurizr-A1_SystemContext.png)

## Deployments

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
