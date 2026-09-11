# Mock PRS (Pseudoniemendienst) for local docker compose.
#
# Build context is the repository root: the mock lives in the main Go module,
# so go.mod/go.sum at the root are needed to build it.
#
#   docker build -f mock-components/prs/Dockerfile .
FROM golang:1.26.1-alpine AS builder

WORKDIR /src

# The main module has a replace directive pointing at test/testdata, so that
# module's go.mod must be present before go mod download will resolve.
COPY go.mod go.sum ./
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /out/mock-prs ./mock-components/prs/cmd

FROM alpine:3.22.0

RUN adduser -D -u 10000 mockprs
USER mockprs

COPY --from=builder /out/mock-prs /usr/local/bin/mock-prs

# SANDBOX_LISTEN_ADDR is deliberately not set here: the unauthenticated
# sandbox helper is enabled by the compose project, which keeps its port
# inside the project network. PUBLIC_URL is required and environment-specific,
# so it has no default either.
ENV LISTEN_ADDR=":8080"
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/mock-prs"]
