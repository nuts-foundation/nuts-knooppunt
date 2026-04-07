# Test Data and Environments

This document describes the external systems that provide identity data, and the constraints that test data must
satisfy.

## Identity Attributes

### URA

The care organization identifier (URA) must match across systems, otherwise:

- the EHR won't be able to find the mCSD resources (e.g. HealthcareService or Endpoint) to exchange data
- authorization at the PDP will fail, because Consent has been registered by/for a different organization
- authorization of access token request will fail, because URA from DeziUserCredential differs from care organization
  X509Credential
- registration at NVI will fail, because the client certificate carries a different URA than the one the organization is
  known under in LRZa
- looking up data at NVI/PRS will fail, because the requesting organization's URA is not recognized
- invocations of PRS will fail, because the client certificate URA does not match the registered organization

| System/Feature  | Relevance of URA                                                                         | Source of URA                                           |
|-----------------|------------------------------------------------------------------------------------------|---------------------------------------------------------|
| **Dezi**        | Identifies the organization on behalf of which the care giver is acting                  | `abonneenummer` in the Dezi token                       |
| **Nuts AuthN**  | Identifies the care organization in the X509Credential; used to authorize token requests | X509Credential, derived from the UZI Server Certificate |
| **EHR**         | Identifies the EHR's organization; used to look up the correct mCSD resources            | Configured by the EHR vendor                            |
| **NVI and PRS** | Identifies the care organization when registering and querying the referral index        | Client certificate for MinVWS GF services               |
| **LRZa**        | Primary identifier of the care organization in the national registry                     | Authoritative source                                    |
| **Mitz**        | Identifies the subscribing organization in consent registrations                         | URA from the X509Credential presented at subscription   |
| **LSP**         | Identifies care organization in the X509Credential                                       | X509Credential, derived from the UZI Server Certificate |

### UZI number

The UZI or Dezi number is the qualified identifier of a care giver. The UZI number itself doesn't need to correlate with
anything, but the caregiver's organization (by URA), does.

## Environments

### Test

| System      | Environment                                                                                                        | URA constraint                                                |
|-------------|--------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------|
| Dezi        | CIBG Acceptatieomgeving                                                                                            | Fixed — only `90000380`, `90000381`, `90000382` are available |
| Nuts AuthN  | Certificates issued through our [Fake CA](https://github.com/nuts-foundation/go-didx509-toolkit/tree/main/test_ca) | Must match URA used in other systems                          |
| EHR         | Configured by the EHR vendor                                                                                       | Must match URA used in other systems                          |
| LRZa        | [https://lrza-test.nuts-services.nl/](https://lrza-test.nuts-services.nl/) (hosted by consortium)                  | No restrictions apply                                         |
| NVI and PRS | iRealisatie Proeftuin                                                                                              | Must register using a certificate carrying the intended URA   |
| LSP         | AORTA Proeftuin                                                                                                    | Must match URA used in other systems                          |

Notes:

- Dezi: During the PoC phase, we most probably won't align Dezi URAs with the rest of the ecosystem, as:
    - The change management on CIBG Dezi side will probably take too long
    - During the PoCs, only LSP/AORTA-GtK will verify the DeziUserCredential.
      - We can ask them not compare the URA from Dezi token v.s. the URA from the X509Credential
      - Vice versa (LSP querying Nuts), Dezi is out of scope.
- NVI/PRS: vendors are supplied with an LDN certificate, which we assume can be used with any (authorized) URA. This is very practical for vendors using 1 certificate for multiple care organizations.
    - But: it does not appear to work, we're checking with MinVWS how this should work.
    - See [https://github.com/nuts-foundation/nuts-knooppunt/issues/469](https://github.com/nuts-foundation/nuts-knooppunt/issues/469)

During PoC phase, do the following things to line up the URAs:

1. Choose any URA for the organization, make available in the EHR.
2. LRZa:
   2.1. Register the organization in [LRZa](https://lrza-test.nuts-services.nl/) with that URA.
   2.2. Register the mCSD resources in the local mCSD Admin Directory under that URA.
3. Nuts AuthN:
   3.1. Using the [Fake CA](https://github.com/nuts-foundation/go-didx509-toolkit/tree/main/test_ca), issue a UZI Server Certificate with that URA.
   3.2. Using the did:x509 toolkit, issue the X509Credential, and load it into the wallet (Nuts subject) for that particular care organization
4. NVI/PRS:
   4.1. Request a certificate with that URA for the iRealisatie Proeftuin.
   4.2. When requesting an access token, use the URA as `iss` and `sub` claim in the JWT bearer grant token.
5. LSP/AORTA: ???
6. For care giver login via Dezi: use any care giver from the Dezi testset. Their `abonneenummer` will differ from the
   organization URA — coordinate with LSP/AORTA-GtK to disable the URA cross-check between DeziUserCredential and
   X509Credential.

In an ideal world, URAs would line up so we can properly authorize URAs, but we lack the following:

- A more flexible Dezi testset, e.g. each vendor should be able to get a range of URAs they can use.
  - Because this is missing, we can't authorize the URA from Dezi when its token is used in data exchanges
- A more flexible CIBG Test CA that allows the vendor to specify the URA for the certificate (most important), and the organization name (less important).
  - Because this is missing, we use our own Fake CIBG CA. 
- Support for the GIS-VN/LDN certificate in NVI/PRS so vendors can use 1 certificate for all of their care organizations.
