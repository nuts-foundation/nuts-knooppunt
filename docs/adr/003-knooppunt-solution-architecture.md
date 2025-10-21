# Knooppunt Solution Architecture

## Context and Problem Statement

We need to decide on the overall architecture, of how the Knooppunt fits into existing or new infrastructure, and how it enables data exchanges.

The Knooppunt helps vendors perform healthcare data exchanges, doing the heavy lifting for:
- Localization (where to find data)
- Addressing (which APIs to call)
- Authentication (who you are, who the other party is)
- Authorization (what can the other party do)
- Consent (did the patient agree to this data exchange)

To do this, it integrates many local (e.g. mCSD Directories), and remote (e.g. NVI, consent registries) data sources and security components (e.g. Nuts node, Open Policy Agent).

### Design Goals
We want a solution that is easy to integrate in varying (existing) environments, without compromising on security and simplicity. 

- **Simplicity**: Easy to deploy and manage, with minimal configuration required. Prevent vendor-specific integration.
- **Flexibility**: Can be adapted to different deployment environments and use cases.
- **Modularity**: Components (e.g. Nuts node, Open Policy Agent) can be enabled/disabled as needed.
- **Security**: Minimize attack surfaces.
- **Pluggability**: Should be as easy as possible to integrate.

## Considered Options
This section describes considered architecture options.

In all of the options, there's a proxy (e.g. NGINX, HAProxy, Traefik) in front of the Knooppunt and/or EHR FHIR API.
This is a typical reverse proxy, handling TLS termination, routing, load balancing, caching, etc.

### Monolithic architecture

The Knooppunt sits on the edge of the vendor's network, handling all data exchanges. It acts as:
- OAuth2 Authorization Server
- Gateway for authorizing and enforcing data exchanges

```text
┌─────────────────┐       ┌────────────┐       ┌──────────────┐
│                 │       │            │       │              │
│ External System ├──────►│ Knooppunt  ├──────►│ EHR FHIR API │
│                 │       │            │       │              │
└─────────────────┘       └────────────┘       └──────────────┘
```

All inbound data requests are routed through the Knooppunt, theoretically offloading all "complicated" concerns from the vendor.

- Advantages:
  - Simple deployment, since it only requires the Knooppunt to be deployed.
  - Easy to configure, since all configuration is centralized in the Knooppunt.
  - Easy to reason about, since all data exchanges go through a single component.
- Disadvantages:
  - Large attack surface on the Knooppunt, since it handles all inbound requests.
    Makes it harder to use security measures vendors already have in place, especially for resource transformation and filtering.
  - Less flexible when the vendor needs additional concerns not supported by the Knooppunt (e.g. auditing, data minimization)

### Nuts Reference Solution Architecture

To fit into the Nuts ecosystem, we could follow the [Nuts Reference Solution Architecture](https://wiki.nuts.nl/books/ssibac/page/referentie-solution-architectuur-wip).
This architecture (following Oasis Service Oriented Architecture), separates the:

- Policy Enforcement Point (PEP), a proxying component that only forwards a request when access decisions indicate that access should be granted.
- Policy Decision Point (PDP), a component that makes access decisions.

Note: Oasis specifies more roles (PIP, PAP), but those are not relevant for this ADR.

The Knooppunt sits as internal service inside the vendor's network. It's only supportive for data exchanges. It acts as:
- OAuth2 Authorization Server
- Policy Decision Point

It relies on a separate, fit-for-purpose Policy Enforcement Point that is either pre-existing or newly deployed.

```text
┌─────────────────┐       ┌──────────────────┐     ┌──────────────┐
│                 │       │                  │     │              │
│ External System ├──────►│       PEP        ├────►│ EHR FHIR API │
│                 │       │                  │     │              │
└─────────────────┘       └────────┬─────────┘     └──────────────┘
                                   │                               
                                   │Authenticate,                  
                                   │Authorize                      
                                   │                               
                          ┌────────▼─────────┐                     
                          │                  │                     
                          │       PDP        │                     
                          │   [Knooppunt]    │                     
                          └──────────────────┘                     
```

The PEP will typically interact with the Knooppunt using OAuth2 Token Introspection (authentication) and the protocol of Open Policy Agent/Authorization API (authorization).

The Knooppunt project can provide a (reference) PEP implementation based on proven, open source software technology (e.g. NGINX or HAProxy), or even an instance of the Knooppunt itself with reduced functionality.

- Advantages:
  - Smaller attack surface on the Knooppunt, since it doesn't handle data exchanges directly.
  - Easier to integrate with existing security infrastructure (e.g. existing reverse proxies that could act as PEP).
  - More flexibility for vendors to choose or reuse a Policy Enforcement Point that fits their needs.
  - Easier to align with vendor compliancy requirements.
- Disadvantages:
  - More complex deployment, since it requires an additional component (the PEP).
  - More complex configuration, since the proxy needs to be set up correctly to work with the Knooppunt.
  - Potentially more points of failure, since there are more components involved.

## Decision Outcome

We have decided to follow the Nuts Solution Architecture, which means we'll separate the PEP and PDP responsibilities.
The Knooppunt will act as PDP for authentication and authorization decisions, while relying on an external PEP to enforce those decisions.

This approach was chosen because it minimizes the attack surface of the Knooppunt, leverages existing security components, and provides flexibility for vendors to use their preferred Policy Enforcement Point (PEP).
While it introduces some deployment and configuration complexity, the benefits in terms of security, modularity, and compliance alignment outweigh these drawbacks.

### Future Reconsideration

If, in the future, there are parties that wish to use the Knooppunt as Policy Enforcement Point,
we could support that by implementing PEP functionality in the Knooppunt. However, it would be just like any other subsystem in the Knooppunt, that can be enabled/disabled as needed. 