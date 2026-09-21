# Context: TA Notified Pull harmonisation & Twiin fall release 1.6

> Sources (translated from Dutch):
> - https://wiki.nuts.nl/books/kring-techniek/page/harmonisatie-ta-notified-pull
> - https://wiki.nuts.nl/books/kring-techniek/page/twiin-najaarsrelease-16

## Harmonisatie TA Notified Pull (Nuts wiki)

The Twiin Agreement Framework contains a 2024 Technical Agreement Notified Pull (TA NP). Via the
National Coverage Network (LDN) programme, Twiin has a mandate to harmonise it into a **generic
building block**, with a subworking group under the Working Group TA (WGTA). Nuts participates since
spring 2026 (Steven van der Vegt lead, Dirk Geurs second). The harmonisation is split into a
**notification component** (developed first as TA Notifications) and a **workflow component**.

Relevance for Nuts:
- The harmonised TA NP becomes a generic building block of the national coverage network.
- Scope goes beyond notifications: addressing, localisation, consent, authorisation,
  identification/authentication are all touched. AuthN/AuthZ is the heavyweight part (Nuts uses
  Verifiable-Credential-based roles/claims; Twiin's trust model differs) — now handled separately by VWS.
- Application-level profiling remains needed after the TAs (e.g. an eOverdracht application profile;
  ownership unclear).
- Impacts eOverdracht standard renewal (part of Nuts v6 migration, managed by the Applications group).
- The result lands in **Twiin release 1.6**, replacing the 2024 version.

Open substantive issues (as recorded):
- Authorisation is in-scope but unelaborated in the TA Notifications concept, despite being critical
- Notification **intent** is unknown when only identifiers are transmitted; unclear how intent becomes
  apparent for generic resources via addressing endpoints
- Document scope keeps shifting on id-only support and receiver-initiated subscriptions
- Subscription topic aggregation levels (L1–L4) presented; normative level selection pending
- Ownership of application-profile development above the TAs unassigned

Planning (2026):
| When | What |
|------|------|
| Weekly, Tuesday morning | Harmonisation TA NP subworking group |
| 24 Aug | Impact-analysis responses from eOverdracht participants |
| 27 Aug | TTA Notifications v0.6 published in the Twiin ontwikkelsupplement |
| September | **TTA Notifications v0.6 POC by COT (delivery 21 Sep)** ← this assignment |
| Week 37 | Initial Twiin release 1.6 information |
| 5–30 Oct | Twiin release 1.6 consultation |
| From 14 Dec | Release 1.6 publication with harmonised TA NP |

Subworking-group participants besides Nuts: VZVZ, Chipsoft, Nedap, Nexus, Cumuluz, VWS.
Twiin contacts: Anneke Huisman, Arnaud Poelman (content); Siu Luc, Jelmer Kranenburg (WGTA).

## Twiin najaarsrelease 1.6 (Nuts wiki)

Twiin releases twice yearly with consultation rounds. Release 1.6 preparation started July 2026.
Released content becomes normative for parties conforming to the Twiin framework.

Significance:
1. The **harmonised TA Notified Pull** targets 1.6; the 2024 version will be removed.
2. Spring release 1.5.0 already directly affected the circle's domain (generic-function requirements,
   revised Notified Pull communication patterns, Safe Network requirements).

Assignment (proposed, Architecture Workgroup as venue): submit a consultation response covering the
harmonised TA NP + communication patterns, generic-function requirements, and how the framework
references external agreements. Out of scope: operational/organisational/security requirements,
care-application-specific standards, Twiin services/processes, editorial choices.

**Input for the consultation response includes the findings of this POC** ("findings from POC TTA
Notifications v0.6 contain specification errors and gaps for Twiin submission").

Timeline: consultation 5–30 Oct, two online sessions in October, expert session week 46, consultation
tables week 48, final publication from 14 Dec.

Links: consultation platform https://consultatie.twiin.nl · normative framework
https://afsprakenstelsel.twiin.nl
