# Build context must be the repo root: the sandbox app is part of the root Go
# module (see README.md's "Run" section for the `go run`/`go test` path).
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /gf-sandbox ./sandbox/app

FROM alpine:3.22.0
COPY --from=builder /gf-sandbox /gf-sandbox

RUN adduser -D -H -u 18091 sandbox-usr
RUN chown 18091 /gf-sandbox
USER 18091:18091

# 3001 is De Zonnebloem's EHR, served when ZONNEBLOEM_FHIR_BASE_URL is set.
EXPOSE 8091 3001
ENTRYPOINT ["/gf-sandbox"]
