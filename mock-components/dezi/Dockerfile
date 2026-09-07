# Build context must be the repo root: this service is part of the root Go module.
FROM golang:1.26.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download && go mod verify

# Only this package's own source is copied, rather than the whole root context.
# The package imports nothing else from this module (`go list -deps` resolves
# no other nuts-knooppunt path), so a root-wide `COPY . .` would add no build
# input while capturing every private key and runtime data directory present
# in the context into a reusable builder layer. An allowlist keeps new key
# locations out by default instead of requiring a matching .dockerignore rule.
COPY ./mock-components/dezi ./mock-components/dezi
RUN CGO_ENABLED=0 go build -o /mock-dezi ./mock-components/dezi

FROM alpine:3.22.0
COPY --from=builder /mock-dezi /mock-dezi

RUN adduser -D -H -u 18092 dezi-usr
RUN chown 18092 /mock-dezi
USER 18092:18092

EXPOSE 8092 8443
ENTRYPOINT ["/mock-dezi"]
