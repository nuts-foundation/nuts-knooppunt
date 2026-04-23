# SMART on FHIR Scope Handling in Nuts Node and PDP

## Context and Problem Statement

Healthcare data exchanges in the Netherlands increasingly involve two distinct scope systems:

- **Nuts use-case scopes**: High-level, use-case-level identifiers (e.g. `medicatieoverdracht`, `bgz`) used in the Nuts
  ecosystem to identify a healthcare data exchange use case.
- **SMART on FHIR (SoF) scopes**: Fine-grained, resource-level access scopes (e.g. `patient/MedicationRequest.read`)
  used in the AORTA-on-FHIR ecosystem, as defined by the SMART on FHIR specification.

As long as only Nuts-side systems query AORTA data sources, there is no issue: the Nuts node issues tokens with
Nuts-level use-case scopes and the PDP evaluates access based on those. However, when **Nuts data holders** receive
requests from systems using AORTA-on-FHIR, the situation is reversed: incoming access tokens carry SMART on FHIR scopes
instead of Nuts use-case scopes.

The Knooppunt's Nuts node and PDP are currently unable to handle SMART on FHIR scopes:

- The Nuts node limits access tokens to a single scope and only supports static scope values.
- The PDP maps scope strings directly to named OPA policy packages (e.g. scope `bgz` → package `bgz`) and has no
  mechanism to match a dynamic SoF scope such as `patient/MedicationRequest.read`.

We need to decide how to make the Knooppunt work correctly as a data holder when the requester presents SMART on FHIR
scopes.

## Considered Options

### Option A: Regex scope matching + dedicated use-case scope (Proposal A from the issue)

This two-part option keeps SMART on FHIR scopes unchanged in the token and adds a bridge inside the Knooppunt.

**Part 1 – Regex scope matching in the Nuts node**

Extend the scope-to-access-policy map in the Nuts node to support regular expressions. A single regex such as
`^(system|user|patient)/[A-Za-z]+\.(read|write|\*)$` can match any SMART on FHIR scope. When a token is introspected,
the Nuts node evaluates each presented scope against the registered patterns. If any scope cannot be matched, the entire
request is rejected.

This effectively allows multiple scopes to be carried in a single token (one Nuts use-case scope and one or more SoF
scopes), resolving the "single scope" limitation.

**Part 2 – Add `medicatieoverdracht` as a coordinated scope**

Add `medicatieoverdracht` (or another agreed use-case identifier) to the set of scopes that both Nuts and AORTA
include in the token's scope claim. The PDP can then map that well-known scope to an existing OPA policy package
(`medicatieoverdracht`) that in turn may inspect the accompanying SoF scopes to make fine-grained access decisions.

- **Advantages:**
  - Minimal change to the existing Knooppunt PDP: only the `medicatieoverdracht` OPA policy needs to be extended.
  - The use-case scope acts as a stable, human-readable anchor in the token that clearly identifies the data exchange
    context.
  - SoF scopes remain in the token unmodified, preserving interoperability with AORTA systems.
  - A single coordinating scope makes audit trails easier to read.
- **Disadvantages:**
  - Requires agreement between the Nuts and AORTA ecosystems to add `medicatieoverdracht` to the SoF scope list.
  - The Nuts node must be changed to support regex-based scope matching and multi-scope tokens.
  - The OPA policy for `medicatieoverdracht` must be extended to inspect and validate SoF scopes present in the token.

### Option B: Scope translation layer

Introduce a translation component (inside the Knooppunt or as a separate adapter) that converts incoming SMART on FHIR
scopes to Nuts use-case scopes before the PDP evaluates the request. The PDP would see only normalised Nuts-level
scopes.

- **Advantages:**
  - No changes to the Nuts node or PDP core logic; scopes remain static strings.
  - Keeps the PDP policy model simple.
- **Disadvantages:**
  - Translation logic is inherently brittle: a single SoF scope (e.g. `patient/*.read`) does not unambiguously map to
    one use case.
  - Information is lost: the PDP policy can no longer inspect the original SoF scopes to apply fine-grained resource
    restrictions.
  - Extra moving part with its own failure modes.

### Option C: Reject SMART on FHIR scopes (do nothing)

Decide that Knooppunt data-holder deployments are out of scope and leave the current behaviour unchanged. Requestors
with SMART on FHIR scopes will receive an authorisation error.

- **Advantages:**
  - No implementation effort.
- **Disadvantages:**
  - Blocks the Knooppunt from being deployed as a data holder in AORTA-on-FHIR scenarios.
  - Contradicts the goal of supporting interoperability between Nuts and AORTA.

## Decision Outcome

Chosen option: **Option A** (regex scope matching in the Nuts node combined with a coordinated `medicatieoverdracht`
use-case scope).

This option is chosen because it:

- Preserves full SoF scope information throughout the authorisation flow, enabling fine-grained policy evaluation.
- Requires targeted, well-understood changes to the Knooppunt while avoiding a fragile translation layer.
- Aligns with the existing policy model: the PDP continues to select a policy package by scope name and delegates the
  fine-grained access decision to OPA.
- Provides a clear integration contract for AORTA parties: they must include `medicatieoverdracht` in the token
  alongside their SoF scopes.

### Consequences

The following changes are required to implement this decision:

1. **Nuts node**: extend the scope-to-access-policy map to support regular expressions so that SMART on FHIR scopes are
   recognised and accepted. If any scope in the token cannot be matched by a registered pattern, the token request must
   be rejected.
2. **Nuts node**: lift or relax the current single-scope limitation to allow a token to carry both the Nuts use-case
   scope (e.g. `medicatieoverdracht`) and one or more SoF scopes simultaneously.
3. **PDP `medicatieoverdracht` policy**: extend the OPA policy to optionally inspect the SMART on FHIR scopes present in
   `input.subject.scope` and use them as additional constraints on the allowed FHIR interactions.
4. **Ecosystem alignment**: reach agreement with the AORTA-on-FHIR programme to include `medicatieoverdracht` in the
   scopes issued alongside the SoF scopes for the Medicatieoverdracht use case.

### Notes on the `medicatieoverdracht` OPA policy extension

The extended policy _could_ inspect the accompanying SoF scopes to restrict access further — for example, denying a
`MedicationRequest` write when the token only carries `patient/MedicationRequest.read`. This keeps the fine-grained
resource-level authorisation logic inside the existing OPA framework rather than in the Nuts node.

### Future considerations

- Once the pattern for `medicatieoverdracht` is proven, the same approach can be applied to other AORTA-on-FHIR use
  cases (e.g. `bgz`, `pzp_gf`).
- If the Nuts node is extended to support regex scopes generically, other ecosystems beyond AORTA may also benefit.
