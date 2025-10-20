The NVI contains information about treatment relationships between patients and healthcare providers.
This information is considered sensitive and should be handled as regular health data.
The NVI is a shared service, but operates on the behalf of each individual healthcare organization that uses it for discovering treatment relationships.
Therfor before sharing this data with other organizations, the NVI has to ensure that the patient has given consent for sharing this information with the requesting organization.

The NVI has to check with the responsible custodian of the patient information before sharing any treatment relationship information.

For this check we need to design an API that allows the NVI to check for patient consent with the responsible custodian.

## API Design for Consent Check

The decision to share information on an API is often made by a component named a Policy Decision Point (PDP).

The PDP is responsible for making authorization decisions based on policies defined by the responsible custodian.

A generic flow for API decision making with a PDP is as follows:

```ascii
┌────────┐        ┌─────┐       ┌────────────────┐
│        │        │     │       │                │
│ Client ├───────►│ PEP ├──────►│ ResourceServer │
│        │        │     │       │                │
└────────┘        └──┬──┘       └────────────────┘
                     │
                     │
                  ┌──▼──┐        ┌────────┐
                  │     │        │        │
                  │ PDP │◄───────┤ Policy │
                  │     │        │        │
                  └─────┘        └────────┘
```

The client requests access to a resource on the Resource Server via the Policy Enforcement Point (PEP).
The PEP intercepts the request and queries the PDP to determine whether access should be granted based on the defined policies.
The PDP evaluates the request against the policies and returns a decision to the PEP, which then enforces that decision by either allowing or denying access to the resource.

This specification will focus on the API design between the PEP and the PDP for checking patient consent.

### Policy decisions in the NVI

The NVI is a bit different from a typical resource server scenario, as the NVI first has to find the matching treatment relationships before it can query the custodian specific PDP.

Also, since the NVI is a shared and centralized service, the custodian's PDPs are external systems that the NVI has to communicate with.
These PDPs should be available for the NVI, and possible other clients. From this perspective, a PDP has to be protected as a regular resource server with authentication and authorization.

If we expand on the above diagram to include the custodian specific components for authentication and autorization, we get the following flow:

```ascii

┌────────┐        ┌─────┐       ┌───────┐
│        │   1    │     │  2    │       │
│ Client ├───────►│ PEP ├──────►│  NVI  │
│        │        │     │       │       │
└────────┘        └─┬┬──┘       └───────┘
                    ││
                    ││
                    ││
                    ││           ┌─────┐      ┌─────────┐
                    ││     4     │     │  5   │         │
                    │└──────────►│ PEP ├─────►│ PDP(RS) │
                    │            │     │      │         │
                    │            └─────┘      └─────────┘
                    │
                    │            ┌─────┐
                    │      3     │     │
                    └───────────►│ AS  │
                                 │     │
                                 └─────┘
```

Where the steps are as follows:

1. The client makes a request to the NVI which is protected by a PEP.
2. The PEP forwards the request to the NVI after successful authentication and authorization of the client.
   The NVI queries for organisations with matching datasets and retrieves the responsible custodian for the patient.
3. The NVI's PEP authenticates itself as the NVI-PEP client to the custodian's Authorization Server (AS) and requests an access token for the PDP.
4. The NVI's PEP makes a decision request to the custodian's PDP which is protected by the custodian's PEP. If the client is authorized, the request is forwarded to the PDP (which acts as a resource server).
5. The PDP evaluates the request based on the defined consent policies and returns a decision to the NVI's PEP. If consent is granted, the NVI can proceed to share the treatment relationship information with the requesting client.

## Proposed standardized API

There is a standardized API for PDP decision requests in development by the OpenID Foundation called AuthZEN. The latest working draft 04 can be found [here](https://openid.github.io/authzen/).

The AuthZEN specification defines a RESTful API for making authorization decisions and allows the PEP to send detailed information about the access request to the PDP.

### Consent Decision Request

The NVI has to provide the folling information to the custodian's PDP in order to check for patient consent:

- Subject: The patient for whom the treatment relationship information is being requested.
- Practitioner: The identifier of the healthcare practitioner requesting the information.
- Organization: The identifier and type of the organization requesting the information.

Furthermore, the NVI has to specify which records it wants to disclose to the requesting organization.

To include multiple records in a single request, we can use the "evaluations" array defined in the AuthZEN specification from section 7.

Since all evaluations should be performed, we assume the default `"evaluations_semantic": "execute_all"` option.

Based on the AuthZEN specification, the following JSON structure can be used for the consent decision request:

```json
{
  "subject": {
    "type": "<urn:oid:system-oid>",
    "id": "<NVI-ID>",
    "properties": {
      "patientId": "<pseudonymized-patient-id>",
      "practitionerId": "<identifier-of-requesting-practitioner>",
      "organization": {
        "id": "<identifier-of-requesting-organization>",
        "type": "<Hospital-code>"
      }
    }
  },
  "action": {
    "name": "disclose"
  },
  "evaluations": [
    {
      "resource": {
        "id": "<uuid-of-resource-to-access>",
        "type": "DocumentReference",
        "properties": {
          "category": "mental-health"
        }
      }
    },
    {
      "resource": {
        "id": "<uuid-of-resource-to-access>",
        "type": "DocumentReference"
        "properties": {
          "category": "medication"
        }
      }
    }
  ]
}
```

### Consent Decision Response

The response from the PDP contains the decision for each evaluation in the requested order.

```json
{
  "evaluations": [
    {
      "decision": false
    },
    {
      "decision": true
    }
  ]
}
```
