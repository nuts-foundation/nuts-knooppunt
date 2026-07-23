# Build context must be the repo root: the sandbox app is part of the root Go
# module (see README.md's "Run" section for the `go run`/`go test` path).
FROM golang:1.26.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gf-sandbox ./sandbox/app

FROM alpine:3.22.0
COPY --from=builder /gf-sandbox /gf-sandbox

RUN adduser -D -H -u 18091 sandbox-usr
RUN chown 18091 /gf-sandbox
USER 18091:18091

EXPOSE 8091
ENTRYPOINT ["/gf-sandbox"]
