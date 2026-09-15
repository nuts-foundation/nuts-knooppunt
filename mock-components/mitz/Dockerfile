# Mock MITZ closed-question service for local docker compose.
#
# Build context is the repository root: mitzmock lives in the main Go module, so
# go.mod/go.sum at the root are needed to build it.
#
#   docker build -f mock-components/mitz/Dockerfile .
FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# The main module has a replace directive pointing at test/testdata, so that
# module's go.mod must be present before go mod download will resolve.
COPY go.mod go.sum ./
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/mitzmock ./mock-components/mitz/cmd

FROM alpine:3.22.0

RUN adduser -D -u 10000 mitzmock
USER mitzmock

COPY --from=builder /out/mitzmock /usr/local/bin/mitzmock

ENV LISTEN_ADDR=":8080"
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/mitzmock"]
