# Build context must be the repo root: this service is part of the root Go module.
FROM golang:1.26.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -o /mock-dezi ./mock-components/dezi

FROM alpine:3.22.0
COPY --from=builder /mock-dezi /mock-dezi

RUN adduser -D -H -u 18092 dezi-usr
RUN chown 18092 /mock-dezi
USER 18092:18092

EXPOSE 8092 8443
ENTRYPOINT ["/mock-dezi"]
