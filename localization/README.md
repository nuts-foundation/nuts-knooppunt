# Localization

## HAPI Interceptor

The hapi interceptor converts pseudonym tokens into registry specific pseudonyms and vice versa.
It hooks into the read, search and response steps of the HAPI request lifecycle.
It uses an external pseudonymization service to perform the conversions. It assumes the service runs at the docker host at port 8082.

### Building

To build the interceptor, run the following command from the hapi-interceptor directory:

```shell
mvn clean compile
```
