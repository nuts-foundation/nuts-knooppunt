# Flexible Authorization Model

## Context and Problem Statement

As we're opening up the Nuts ecosystem to other governance domains, the authorization model needs to be flexible enough to accommodate different types of actors and permissions.
The current model (Nuts spec v2) is primarily designed around single, fixed per-use case OAuth2 scopes, but we need to consider how to make it more flexible without compromising security or usability.
For instance, for interoperability with LSP/AORTA-on-FHIR we need to support more dynamic SMART on FHIR scopes.

### Nuts v2 Authorization Model

Nuts (v2) separates:

- When an access token is requested:
  - authentication:
    - who is the entity performing the request?
    - organization identity
    - end-user identity
    - to be extended with client/node/gateway identity
  - authorization:
    - may this authenticated entity participate in this use case?
      - e.g., is the care organization and the software they're using qualified? 
- When the access token is used (data access):
  - authorization:
    - do the business rules for this use case allow this interaction?
    - is the entity allowed to perform this interaction?
      - e.g.:
        - Did the patient give consent?
        - Is the requester a healthcare provider involved in the patient's care?
        - Is the requester the fulfiller of the FHIR Task being updated?

The catch is in the "use case" rules; parties are authorized to perform certain interactions in the context of a single use case, as requested by the client.

Due to this model, the client can't:

- use an access token in multiple use cases, even if the use case authorization rules overlap (e.g. both allow reading a Task resource).
- request an access token for multiple use cases

### SMART on FHIR Authorization Model

SMART on FHIR authorization uses flexible OAuth2 scopes that can be defined by the client, which are use case agnostic.
For example, a client can request the `patient/Task.r` scope to read Task resources, regardless of the specific use case.

## Considered Options

This section describes the considered options for addressing the authorization model challenge.

### Extend Nuts Authorization Model with Flexible Scopes

In this option, we would extend the Nuts authorization model to support flexible scopes similar to SMART on FHIR.

This would allow use case writers to use SMART on FHIR scopes, instead of defining their own use case specific scopes.

The Nuts specification would need to accommodate this by allowing flexible evaluation of requested scopes.
The current approach uses a fixed set of credential requirements specified by a Presentation Definition, which is not flexible enough to support this.

This flexibility could be achieved by making this configurable in the Nuts node, or integrating callbacks to a policy engine (e.g. Open Policy Agent in the Knooppunt PDP).
Integration with Open Policy Agent would also converge the Nuts access policy model (using Presentation Definitions) with the authorization policy model used in the Knooppunt PDP,
increasing consistency and allowing policies to be 1 package.

**Advantages:**
- A single access token can potentially cover multiple interactions (e.g. reading different resource types), reducing the number of token requests.
- Aligns the Nuts ecosystem with widely adopted industry standards, improving interoperability with existing FHIR tooling and EHR systems.
- Flexible scope evaluation can be delegated (e.g. to Open Policy Agent), keeping the Nuts node policy-agnostic and adaptable.

**Disadvantages:**
- Significant changes to the Nuts specification and node implementation are required, introducing risk and development effort.
- Flexible scopes are harder to reason about from a governance perspective; use case-specific guardrails become less explicit.
- A more complex scope evaluation mechanism increases the attack surface and makes authorization audits more difficult.
- If a PDP isn't built-in to the Nuts node, it requires deploying an additional component (policy evaluator), increasing operational complexity.
  - Existing v2 use cases (Huisartsinzage, Thuismonitoring) can rely on what's currently available in the Nuts node
  - New use cases that want to use flexible scopes can integrate with the PDP for scope evaluation.

#### Impact

- As we're dropping the fixed set of credential requirements, we need to specify how to map credentials to identities (e.g. end-user, organization, client) in a flexible way.
  - This could be done using an Open Policy Agent/Rego policy as well, allowing flexible mapping of credentials to identities.
- Specify protocol for evaluating flexible scopes, either through configuration or integration with a policy engine.
- Devise a migration path for existing v2 use cases, or keep it backwards compatible by supporting both Nuts access policy/presentation definitions and dynamic scope evaluation.

### Combine use case specific scopes with flexible scopes

In this option, we would have use case writers specify both use case specific scopes and flexible scopes in their use case, if interoperability with SMART on FHIR is desired.

Implementing parties that support use case level scopes can use those for authorization, while those that want to support SMART on FHIR can use the flexible scopes.

**Advantages:**
- Interoperability with SMART on FHIR-based systems is possible without forcing all parties to migrate to a new model.

**Disadvantages:**
- Use case writers bear additional burden: each use case must be maintained in two representations (use case specific + SMART on FHIR scopes).
- Risk of divergence between the two scope sets over time, potentially leading to inconsistent authorization behavior.
- Increased complexity in the authorization and token issuance logic to handle both scope types simultaneously.
- Partial adoption may fragment the ecosystem, where some use cases support both models and others do not.

### Require other authorization models to adapt to the Nuts model

**Advantages:**
- Simplest approach from a Nuts specification and implementation perspective; no changes to the Nuts' authorization model are needed.
- Authorization remains strongly tied to use cases, preserving the governance guarantees that are central to the Nuts trust model.
- Easier to audit and reason about: every access token maps to an explicit, well-defined use case.
- No risk of scope ambiguity or overlap between different governance domains.

**Disadvantages:**
- Creates a high adoption barrier for parties already invested in SMART on FHIR or other established authorization models.
- LSP/AORTA-on-FHIR interoperability goals cannot be met without significant adaptation effort on the other party's side.
- May limit the growth of the Nuts ecosystem by excluding use cases and integrations that rely on flexible scopes.
- Positions Nuts as a closed standard, potentially reducing credibility and adoption in the broader health IT landscape.

## Decision Outcome
