# Mock PRS

A stand-in for the national pseudonymization service (PRS, "Pseudoniemendienst")
as the Proeftuin acceptance environment is documented to run it, built for the
GF Sandbox and for the tests in `component/pseudonymisation`. **This is not a
PRS implementation and must not be treated as one.** The package comment in
`prsmock.go` says exactly which behaviour of PRS v0.0.18 it reproduces and
which it leaves out; `docs/prs-contract.md` puts that version next to the
service's newer main branch and the draft implementation guide.

## What it serves

- `POST /oprf/eval` over TLS, requiring a client certificate on the handshake
  and a bearer token, with v0.0.18's request validation, error bodies, a real
  OPRF evaluation on ristretto255 and a JWE for the recipient.
- `POST /oauth/token` on the same listener: a stand-in for the ministry's
  token endpoint. It verifies the JWT-bearer assertion `component/authn`
  sends against the presented client certificate, requires the assertion to
  name this endpoint as its audience and the posted scope and target audience
  to equal the signed ones, and issues an opaque token that `/oprf/eval` then
  requires, presented with the same certificate and requested for this
  service, as the real service's `cnf` and audience checks demand. The scope
  value itself is not checked, as v0.0.18 does not check it. It does not know
  the ministry's keys, so it checks nothing the real endpoint would check
  beyond the assertion itself.
- `POST /sandbox/finalize` and `POST /sandbox/tokenize` on a second,
  plaintext listener. The real PRS has neither. They exist because the
  sandbox NVI, a HAPI server with an interceptor, cannot decrypt a JWE and
  de-blind on ristretto255 itself: finalize turns a Knooppunt identifier
  value into the recipient's pseudonym (hex, so it fits a FHIR id), tokenize
  turns a pseudonym back into a fresh identifier value for an audience. The
  listener has no authentication: whoever reaches it can de-tokenize any
  identifier value and mint identifier values for any pseudonym and audience.
  It is off unless `SANDBOX_LISTEN_ADDR` is set; the compose project sets it
  and publishes no port for it, so only that project's containers reach it.

## Deliberate deviations

- One recipient key pair serves every registered recipient; the real service
  holds a key per organization and scope. In the sandbox the mock holds the
  NVI's private key, since it de-blinds on the NVI's behalf.
- Any client certificate is accepted; the acceptance ingress verifies a UZI
  certificate.
- Nothing cryptographic is committed. `./sandbox/generate-demo-certs.sh`
  writes the TLS certificate, the recipient key and the OPRF key to the
  gitignored `sandbox/.certs/`. Without `RECIPIENT_KEY_FILE` and
  `OPRF_KEY_FILE` the mock generates both, and every pseudonym the NVI holds
  stops matching on the next restart.

## Run

    go run ./mock-components/prs/cmd
    go test ./mock-components/prs/... ./component/pseudonymisation/...

Environment: `LISTEN_ADDR` (`:8443`), `PUBLIC_URL` (required: the https
origin clients use, the audience and target the token endpoint enforces),
`SANDBOX_LISTEN_ADDR` (unset: helper off), `TLS_CERT_FILE`, `TLS_KEY_FILE`,
`RECIPIENT_KEY_FILE`, `OPRF_KEY_FILE`,
`RECIPIENTS` (`ura:scope` pairs, comma separated, default
`90000901:nationale-verwijsindex`). Containerized, it is the `mock-prs`
service of the sandbox profile in `docker-compose.yml`.
