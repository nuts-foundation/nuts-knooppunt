FROM golang:1.24.4-alpine AS builder

ARG TARGETARCH
ARG TARGETOS

ARG GIT_COMMIT=0
ARG GIT_BRANCH=main
ARG GIT_VERSION=undefined

ENV GOPATH=/

COPY go.mod .
COPY go.sum .
COPY ./test/testdata/go.mod test/testdata/
COPY ./test/testdata/go.sum test/testdata/
RUN go mod download && go mod verify
COPY . .

RUN mkdir /app
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s -X 'github.com/nuts-foundation/nuts-knooppunt/component/status.GitCommit=${GIT_COMMIT}' -X 'github.com/nuts-foundation/nuts-knooppunt/component/status.GitBranch=${GIT_BRANCH}' -X 'github.com/nuts-foundation/nuts-knooppunt/component/status.GitVersion=${GIT_VERSION}'" -o /app/bin .

# alpine
FROM alpine:3.22.0
RUN apk update \
  && apk add --no-cache \
  tzdata \
  curl
COPY --from=builder /app/bin /app/bin

HEALTHCHECK --start-period=30s --timeout=5s --interval=10s \
  CMD curl -f http://localhost:8081/status || exit 1

RUN adduser -D -H -u 18081 app-usr
RUN chown -R 18081 /app
USER 18081:18081

WORKDIR /app

EXPOSE 8080 8081
ENTRYPOINT ["/app/bin"]
