# TTA Notifications v0.6 — POC documentation set

English-language extraction of all documentation relevant to the **POC TTA Notifications v0.6** assignment
(fetched 2026-09-10; original sources are partly in Dutch). These are faithful extractions/translations,
not verbatim copies — always check the linked source for normative wording.

| File | Source |
|------|--------|
| [01-poc-assignment.md](01-poc-assignment.md) | [Nuts wiki: POC TTA Notifications v0.6](https://wiki.nuts.nl/books/kring-techniek/page/poc-tta-notifications-v06) |
| [02-tta-notifications-v06-spec.md](02-tta-notifications-v06-spec.md) | [Twiin ontwikkelsupplement: 10.3.1.1 TTA Notifications (v0.6)](https://ontwikkelsupplement.twiin.nl/actueel/10-3-1-1-tta-notifications-v0-6) — **the normative spec for this POC** |
| [03-gf-ig-notification.md](03-gf-ig-notification.md) | [GF IG (branch): Notification](https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/notification.html) |
| [04-subscription-topics-exploration.md](04-subscription-topics-exploration.md) | [GF IG (branch): Subscription Topics Exploration](https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/subscription-topics-exploration.html) |
| [05-notified-pull.md](05-notified-pull.md) | [GF IG (branch): Notified Pull](https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/notified-pull.html) |
| [06-clinical-workflow.md](06-clinical-workflow.md) | [GF IG (branch): Clinical Workflow (COW profile)](https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/clinical-workflow.html) |
| [07-context-harmonization.md](07-context-harmonization.md) | [Nuts wiki: Harmonisatie TA Notified Pull](https://wiki.nuts.nl/books/kring-techniek/page/harmonisatie-ta-notified-pull) + [Twiin najaarsrelease 1.6](https://wiki.nuts.nl/books/kring-techniek/page/twiin-najaarsrelease-16) |
| [08-comparison-ta-np.md](08-comparison-ta-np.md) | Comparison with [TA Notified Pull v1.0.1](https://www.twiin.nl/tanp) (2024) — answers POC research question 10 |
| [09-knooppunt-vendor-split.md](09-knooppunt-vendor-split.md) | Proposal: knooppunt vs vendor responsibility split — answers POC research questions 3/4 |
| [PLAN.md](PLAN.md) | Implementation plan for the POC in this repository |
| [FINDINGS.md](FINDINGS.md) | POC findings per research question (input for the Twiin 1.6 consultation) |
| [spec-feedback-twiin.md](spec-feedback-twiin.md) / [.pdf](spec-feedback-twiin.pdf) | Concise spec feedback for the Twiin 1.6 consultation, distilled from FINDINGS RQ 1/9 and re-verified against the Backport IG |
| [ARCHITECTURE.md](ARCHITECTURE.md) | How the POC fits together: architecture drawing + sequence diagrams (`docs/tta-notifications-*-sd.puml`) |

Key upstream standards referenced by the spec:

- [HL7 FHIR Subscriptions R5 Backport IG (R4/R4B)](https://hl7.org/fhir/uv/subscriptions-backport/) — conformance target
- [FHIR R5 Subscription Framework](https://build.fhir.org/subscriptions.html)
- [GF IG GitHub repo](https://github.com/nuts-foundation/nl-generic-functions-ig/)

Sources not fetchable without authentication (referenced from the assignment page):

- Integration phase plan & budget, May 2026 (Google Doc, access-restricted)
- Slack threads in #kring-toepassingen / #kring-techniek-pub (eOverdracht impact-analysis call, WGTA notes)
