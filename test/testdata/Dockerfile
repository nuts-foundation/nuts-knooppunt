# Seed program: populates a fresh deployment with demo data. See cmd/main.go
# for what it does. Used by the Kubernetes deployment's seed Job; the local
# docker compose stack runs this module directly with `go run` instead, so it
# doesn't need a rebuild on every change.
#
# Build context is the repository root: the seed needs the demo certificates
# from test/e2e/pep/certs, which live outside this module's own directory.
#
#   docker build -f test/testdata/Dockerfile .
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY test/testdata/go.mod test/testdata/go.sum ./
RUN go mod download

COPY test/testdata/ .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/seed ./cmd

FROM alpine:3.22.0

RUN adduser -D -u 10000 seed
USER seed

COPY --from=builder /out/seed /usr/local/bin/seed
# Only the two organizations' key/chain pairs organizations() actually reads
# - not the CA key, the requester key (a different test's fixture), or the
# cert-generation scripts/README, none of which the seed program uses.
COPY test/e2e/pep/certs/plataan.key test/e2e/pep/certs/plataan-chain.pem test/e2e/pep/certs/zonnebloem.key test/e2e/pep/certs/zonnebloem-chain.pem /certs/

ENTRYPOINT ["/usr/local/bin/seed"]
