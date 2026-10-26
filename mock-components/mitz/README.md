# Mock Mitz

A stand-in for Mitz, the national consent service, built for the e2e tests and the GF Sandbox
(`sandbox/DESIGN.md`). **This is not a Mitz implementation and must not be treated as one.** It answers
the Knooppunt's closed questions, keeps the consent subscriptions the Knooppunt creates, and notifies
them when a patient's consent changes, with just enough of each interface for those callers.

This directory is a library, `package mitzmock`. The command that serves it is in `cmd/`: the
`mitzmock` service in `docker-compose.yml`, and on Kubernetes the `helm/mock-mitz` chart, which the
test environment deploys. Tests start it in process with `NewClosedQuestionService`;
`NewSubscriptionService` is a smaller double that only captures subscriptions.

## Endpoints

| Endpoint | Reached by | Purpose |
| --- | --- | --- |
| `POST /geslotenautorisatievraag/xacml3` | Knooppunt | the closed question, answered with an XACML decision |
| `POST /abonnementen/fhir/Subscription` | Knooppunt | stores a consent subscription |
| `GET /abonnementen/fhir/Subscription` | sandbox | the stored subscriptions, all or those of one `providerid` and `patientid` |
| `DELETE /abonnementen/fhir/Subscription` | reset, recycle | drops every subscription, or those of one `providerid` and `patientid` |
| `PUT /mock/consent/{bsn}` | sandbox | changes one patient's answer and notifies that patient's subscriptions |
| `GET /mock/consent/{bsn}` | sandbox | one patient's answer, and whether it was changed |
| `DELETE /mock/consent/{bsn}`, `DELETE /mock/consent` | reset, recycle | restores the default answer and drops the decision log entries, without a notification |
| `GET /mock/decisions?traceid=` | sandbox | the closed questions answered within one trace |
| `GET /status` | health checks | `ok` |

Everything under `/mock/` exists only on this mock (`sandbox/DESIGN.md` section 10, deviation 3). The
sandbox also uses the `GET` and `DELETE` on subscriptions only against this mock: without one it
reports a subscription as unknown and a reset as partial.

## The answer

Permit by default. A request body containing `bsn:deny` is answered Deny, and one containing
`bsn:error` with HTTP 500; both are looked for in the raw body, before anything else. Then comes the
answer `PUT /mock/consent/{bsn}` stored for the patient the question is about, then the default, which
a test changes with `SetResponse`. One answer covers every provider category.

## Notifications

`PUT /mock/consent/{bsn}` takes `{"decision": "Permit" | "Deny", "changeId": "<lowercase UUID>"}`, the
`changeId` optional, and posts a notification Bundle to every subscription of that patient, one after
the other, within 8 seconds for all of them. The response reports each attempt. The Bundle follows the
`vzvz.fhir.mitz` 3.8.0 example `nl-vzvz-mitz-bundle-notify.json` and is sent as `application/fhir+json`
only. A delivery is not retried and a redirect is not followed. Every change gets a `Consent.dateTime`
strictly later than any before it, so a receiver can order notifications by it; the package gives
that element no such guarantee.

## Decision log

Every closed question is logged with the trace id of its `traceparent` header, the patient, data holder
and requester it names, the decision, and the raw request and response. The last 512 are kept. The GF
Sandbox's viewer reads them by trace id to show Mitz's answer to a source request.

## Deliberate deviations

- No authentication and plain HTTP. Whoever reaches the port can change any patient's answer, read the
  decision log and the subscriptions, and drop them. Local compose publishes it on the host's port
  8087; the Helm chart has no ingress, so only workloads in the cluster reach it.
- One subscription per provider and patient: a second one for the same pair returns the first.
- No notification retries: each subscription gets at most one delivery attempt per change, where Mitz
  retries a notification it got no acknowledgement for until it is delivered (PvE Aansluiting
  Uitwisselingssystemen 3.8.0, AUS-TR-e0150).
- The notification's shape and timing: `sandbox/DESIGN.md` section 10 lists those deviations.

## Run

    go run ./mock-components/mitz/cmd
    go test ./mock-components/mitz/...

Environment: `LISTEN_ADDR` (`:8080`).
