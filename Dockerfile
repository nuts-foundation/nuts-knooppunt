FROM golang:1.24.4-alpine AS builder

ARG TARGETARCH
ARG TARGETOS

ENV GOPATH /

COPY go.mod .
COPY go.sum .
RUN go mod download && go mod verify
COPY . .

RUN mkdir /app
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /app/bin .

# alpine
FROM alpine:3.22.0
RUN apk update \
  && apk add --no-cache \
             tzdata \
             curl
COPY --from=builder /app/bin /app/bin

HEALTHCHECK --start-period=30s --timeout=5s --interval=10s \
    CMD curl -f http://localhost:8080/health || exit 1

RUN adduser -D -H -u 18081 app-usr
USER 18081:18081

WORKDIR /app

EXPOSE 8080
ENTRYPOINT ["/app/bin"]