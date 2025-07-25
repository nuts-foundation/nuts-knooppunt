# nuts-knooppunt
Implementation of the Nuts Knooppunt specifications

## Running
Using Docker:

```shell
docker build . --tag nutsfoundation/nuts-knooppunt:local
docker run -p 8080:8080 nutsfoundation/nuts-knooppunt:local
```

## Endpoints

- Landing page: [http://localhost:8080](http://localhost:8080)
- Health check endpoint: [http://localhost:8080/health](http://localhost:8080/health)

## Go toolchain
It's a typical Go application, so:

```shell
go test ./...
```

and:

```shell
go build .
./nuts-knoopppunt
```