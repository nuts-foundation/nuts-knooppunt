# Mock PRS

A stand-in for the national pseudonymization service (PRS, "Pseudoniemendienst")
as the Proeftuin acceptance environment is documented to run it, built for the
GF Sandbox and for the tests in `component/pseudonymisation`. **This is not a
PRS implementation and must not be treated as one.** The package comment in
`prsmock.go` says exactly which behaviour of PRS v0.0.18 it reproduces and
which it leaves out; `docs/prs-contract.md` puts that version next to the
service's newer main branch and the draft implementation guide.

## What it serves

- `POST /oprf/eval` over plain HTTP, requiring a bearer token, with
  v0.0.18's request validation, error bodies, a real OPRF evaluation on
  ristretto255 and a JWE for the recipient. The acceptance environment is
  behind mTLS, but that is terminated by the ingress in front of the
  service, so the mock assumes it and does not model it. Whether our own
  client presents a certificate is covered by
  `component/pseudonymisation/integration_test.go`, against the real service.
- `POST /oauth/token` on the same listener: a stand-in for the ministry's
  token endpoint as its documentation describes it: the client credentials
  grant with `scope` and `target_audience`, authenticated by mTLS, which is
  assumed to have happened in front of the mock. It issues an opaque token
  that `/oprf/eval` requires and that must have been requested for this
  service (`PUBLIC_URL`). The JWT-bearer `client_assertion` that
  `component/authn` adds is not in that documentation and is ignored; the
  scope value is not checked, as v0.0.18 does not check it.
- `POST /sandbox/finalize` and `POST /sandbox/tokenize` on a second
  listener. The real PRS has neither. They exist because the
  sandbox NVI, a HAPI server with an interceptor, cannot decrypt a JWE and
  de-blind on ristretto255 itself: finalize turns a Knooppunt identifier
  value into the recipient's pseudonym (hex, so it fits a FHIR id), tokenize
  turns a pseudonym back into a fresh identifier value for an audience. The
  listener has no authentication: whoever reaches it can de-tokenize any
  identifier value and create identifier values for any pseudonym and audience.
  It is off unless `SANDBOX_LISTEN_ADDR` is set; the compose project sets it
  and publishes no port for it, so only that project's containers reach it.

## Deliberate deviations

- One recipient key pair serves every registered recipient; the real service
  holds a key per organization and scope. In the sandbox the mock holds the
  NVI's private key, since it de-blinds on the NVI's behalf.
- No transport security at all, where acceptance is reached over mTLS with a
  UZI certificate. The deployment terminates it in front of the mock.
- Nothing cryptographic is committed and nothing runs on the host first: the
  service creates its recipient key and OPRF key on first start at
  `RECIPIENT_KEY_FILE` and `OPRF_KEY_FILE` (the image defaults to `/keys`,
  where the compose project mounts a named volume), so pseudonyms stay
  stable across restarts and `docker compose down -v` starts afresh.

## Run

    go run ./mock-components/prs/cmd
    go test ./mock-components/prs/... ./component/pseudonymisation/...

Environment: `LISTEN_ADDR` (`:8080`), `PUBLIC_URL` (required: the origin
clients use, the audience and target the token endpoint enforces),
`SANDBOX_LISTEN_ADDR` (unset: helper off), `RECIPIENT_KEY_FILE`,
`OPRF_KEY_FILE`,
`RECIPIENTS` (`ura:scope` pairs, comma separated, default
`90000901:nationale-verwijsindex`). Containerized, it is the `mock-prs`
service of the sandbox profile in `docker-compose.yml`.
