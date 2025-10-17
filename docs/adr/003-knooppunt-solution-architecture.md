# Knooppunt Solution Architecture

## Context and Problem Statement

We need to decide on the overall architecture, of how the Knooppunt fits into existing infrastructure, and how it enables data exchanges.

The Knooppunt helps vendors perform healthcare data exchanges, doing the heavy lifting for:
- Localization (where to find data)
- Addressing (which APIs to call)
- Authentication (who you are, who the other party is)
- Authorization (what can the other party do)
- Consent (did the patient agree to this data exchange)

To do this, it integrates many local (e.g. mCSD Directories), and remote (e.g. NVI, consent registries) data sources and security components (e.g. Nuts node, Open Policy Agent).

### Relation to Oasis Service Oriented Architecture
To fit into the Nuts ecosystem, we follow the [Nuts Reference Solution Architecture](https://wiki.nuts.nl/books/ssibac/page/referentie-solution-architectuur-wip).
This architecture (following Oasis Service Oriented Architecture), separates the:

- Policy Enforcement Point (PEP), a proxying component that only forwards a request when access decisions indicate that access should be granted.
- Policy Decision Point (PDP), a component that makes access decisions.

This ADR proposes which and how the Knooppunt fills these roles.

Note that Oasis specifies more roles (PIP, PAP), but those are not relevant for this ADR.

### Design Goals
We want a solution that is easy to integrate in varying (existing) environments, without compromising on security and simplicity. 

- **Simplicity**: Easy to deploy and manage, with minimal configuration required. Prevent vendor-specific integration.
- **Flexibility**: Can be adapted to different deployment environments and use cases.
- **Modularity**: Components (e.g. Nuts node, Open Policy Agent) can be enabled/disabled as needed.
- **Security**: Minimize attack surfaces.
- **Pluggability**: Should be as easy as possible to integrate.

### Use cases

Deciding on the architecture depends on the use cases we want to support. The main use cases are:

- Handling data exchange, initiated by a remote party (inbound)
  - This involves receiving requests, authenticating, authorizing, and responding.
  - It could also involve masking or filtering data based on consent or authorization rules, before it's returned to the requester.
- Initiating data exchange with a system from a remote party (outbound)
    - This involves looking up parties, endpoints, authentication
    - Outbound requests are out of scope for this ADR.

## Considered Options
This section describes considered architecture options.

In any of the options, there's a proxy (e.g. NGINX, HAProxy, Traefik) in front of the Knooppunt and/or EHR FHIR API.
This is a typical reverse proxy, handling TLS termination, routing, load balancing, caching, etc.

### Knooppunt as both PEP and PDP
The Knooppunt sits on the edge of the vendor's network, handling all data exchanges. It acts as:
- OAuth2 Authorization Server
- Policy Decision Point
- Policy Enforcement Point

```text
┌─────────────────┐       ┌─────────┐       ┌────────────┐       ┌──────────────┐
│                 │       │         │       │            │       │              │
│ External System ├──────►│  Proxy  │──────►│ Knooppunt  ├──────►│ EHR FHIR API │
│                 │       │         │       │ [PEP/PDP]  │       │              │
└─────────────────┘       └─────────┘       └────────────┘       └──────────────┘
```

Data exchanges are routed through the Knooppunt, theoretically offloading all "complicated" concerns from the vendor.

- Advantages:
  - Simpler deployment, since it only requires the Knooppunt to be deployed.
  - Easier to configure, since all configuration is centralized in the Knooppunt.
  - Easier to reason about, since all data exchanges go through a single component.
- Disadvantages:
  - Large attack surface on the Knooppunt, since it handles all data exchanges.
    Makes it harder to use security measures vendors already have in place, especially for resource transformation and filtering.
  - Might not actually make things easier for vendors, if they want to implement requirements not supported by the Knooppunt (e.g. auditing, data minimization)

### Knooppunt as PDP, separate PDP
The Knooppunt sits as internal service inside the vendor's network. It's only supportive for data exchanges. It acts as:
- OAuth2 Authorization Server
- Policy Decision Point

It relies on a separate, fit-for-purpose Policy Enforcement Point that is either pre-existing or newly deployed.
The Knooppunt project can provide a reference implementation based on proven, open source software technology (e.g. NGINX or HAProxy).

```text
┌─────────────────┐       ┌──────────────────┐     ┌──────────────┐
│                 │       │                  │     │              │
│ External System ├──────►│      Proxy       ├────►│ EHR FHIR API │
│                 │       │      [PEP]       │     │              │
└─────────────────┘       └────────┬─────────┘     └──────────────┘
                                   │                               
                                   │Authenticate,                  
                                   │Authorize                      
                                   │                               
                          ┌────────▼─────────┐                     
                          │                  │                     
                          │    Knooppunt     │                     
                          │      [PDP]       │                     
                          └──────────────────┘                     
```

- Advantages:
  - Smaller attack surface on the Knooppunt, since it doesn't handle data exchanges directly.
  - Easier to integrate with existing security infrastructure (e.g. existing reverse proxies).
  - More flexibility for vendors to choose or reuse a Policy Enforcement Point that fits their needs.
  - Easier to align with vendor compliancy requirements.
- Disadvantages:
  - More complex deployment, since it requires an additional component (the proxy).
  - More complex configuration, since the proxy needs to be set up correctly to work with the Knooppunt.
  - Potentially more points of failure, since there are more components involved.

### Variant for outbound data exchanges
Outbound data exchanges, initiated by the local EHR, could be routed through the Knooppunt. This could offload the EHR from:

- Negotiating TLS with the external party
- Negotiating authentication with the external party
- Looking up the right endpoint to call

This is not in scope for this ADR.

## Decision Outcome

We have decided to implement the Knooppunt as a Policy Decision Point (PDP), option "Knooppunt as internal system";
integrating with existing infrastructure via an external proxy for inbound data exchanges.
This approach was chosen because it minimizes the attack surface of the Knooppunt, leverages existing security components, and provides flexibility for vendors to use their preferred Policy Enforcement Point (PEP).
While it introduces some deployment and configuration complexity, the benefits in terms of security, modularity, and compliance alignment outweigh these drawbacks.

### Future Reconsideration

If, in the future, there are parties that wish to use the Knooppunt as Policy Enforcement Point (the "Knooppunt as both PEP and PDP" option),
we could support that by implementing PEP functionality in the Knooppunt. However, it would be just like any other subsystem in the Knooppunt, that can be enabled/disabled as needed. 