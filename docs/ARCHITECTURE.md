# Architecture
This document details the inner design of the Knooppunt.

![structurizr-GF_SystemContext.svg](images/structurizr-GF_SystemContext.svg)

## Addressing

![structurizr-GF_Addressing_ComponentDiagram.svg](images/structurizr-GF_Addressing_ComponentDiagram.svg)

## Localization

![structurizr-GF_Localization_ComponentDiagram.svg](images/structurizr-GF_Localization_ComponentDiagram.svg)

## Authentication

![structurizr-Authentication_ComponentDiagram.svg](images/structurizr-Authentication_ComponentDiagram.svg)

## Handling inbound data requests

External data requests are authenticated and authorized by the Knooppunt.

### Nuts Reference Solution Architecture

We follow the [Nuts Reference Solution Architecture](https://wiki.nuts.nl/books/ssibac/page/referentie-solution-architectuur-wip) for handling inbound data requests:

<img src="https://wiki.nuts.nl/uploads/images/gallery/2024-05/solution-architecture-1.png" alt="Nuts Reference Solution Architecture"/>

The Knooppunt acts as Authorization Server ("AS") and Policy Decision Point ("PXP").

### Implementation

![structurizr-DataExchange_ComponentDiagram.svg](images/structurizr-DataExchange_ComponentDiagram.svg)