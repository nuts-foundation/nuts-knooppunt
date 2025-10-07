# Integration
This document describes how to integrate with the Knooppunt.

## NVI
The chapter describes how to integrate with the NVI (Nederlandse VerwijsIndex) service using the Knooppunt.

You can create or search for DocumentReference resources using the following endpoints:
- Registration endpoint: [POST http://localhost:8081/nvi/DocumentReference](http://localhost:8081/nvi/DocumentReference)
- Search endpoint:
  - [POST http://localhost:8081/nvi/DocumentReference/_search](http://localhost:8081/nvi/DocumentReference/_search)
  - [GET http://localhost:8081/nvi/DocumentReference](http://localhost:8081/nvi/DocumentReference)

These endpoints need the URA of the requesting care organization. You provide this URA using the `X-Tenant-ID` HTTP header:

```http
X-Tenant-ID: http://fhir.nl/fhir/NamingSystem/ura|<URA>
```