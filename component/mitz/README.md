# MITZ Component

This component generates XACML (eXtensible Access Control Markup Language) authorization decision queries for healthcare access control scenarios.

## Overview

The MITZ component creates SOAP envelopes containing XACML authorization decision queries based on the IHE (Integrating the Healthcare Enterprise) profile for Access Control (APPC).

## Usage

### Creating an XACML Authorization Decision Query

```go
package main

import (
    "fmt"
    "github.com/nuts-foundation/nuts-knooppunt/component/mitz"
)

func main() {
    // Create a request with all required parameters
    req := mitz.XACMLRequest{
        // Resource attributes (about what is being accessed)
        PatientBSN:             "900186021",      // Patient's BSN
        HealthcareFacilityType: "Z3",             // Type of healthcare facility
        AuthorInstitutionID:    "00000659",       // Institution ID that created the document

        // Action attributes (what action is being requested)
        EventCode: "GGC002",                      // Event/procedure code

        // Subject attributes (who is requesting access)
        SubjectRole:            "01.015",         // Healthcare professional role
        ProviderID:             "000095254",      // Healthcare provider ID
        ProviderInstitutionID:  "00000666",       // Institution of the provider
        ConsultingFacilityType: "Z3",             // Type of consulting facility

        // Environment attributes (context of the request)
        PurposeOfUse: "TREAT",                    // Purpose: TREAT, RESEARCH, etc.

        // Endpoint
        ToAddress: "http://localhost:8000/4",     // XACML PDP endpoint
    }

    // Generate the XACML query
    xml, err := mitz.CreateXACMLAuthzDecisionQuery(req)
    if err != nil {
        panic(err)
    }

    fmt.Println(xml)
}
```

## XACML Request Structure

The generated SOAP envelope contains:

### SOAP Header
- **Action**: XACMLAuthorizationDecisionQueryRequest
- **MessageID**: Unique UUID for the request
- **ReplyTo**: Anonymous endpoint
- **To**: Target endpoint address

### SOAP Body - XACML Request
The request is organized into four categories of attributes:

#### 1. Resource Attributes
Information about the resource being accessed:
- **resource-id**: Patient BSN (InstanceIdentifier)
- **healthcare-facility-type-code**: Type of facility (CodedValue)
- **author-institution:id**: ID of authoring institution (InstanceIdentifier)

#### 2. Action Attributes
Information about the action being performed:
- **event-code**: Event/procedure code (CodedValue)

#### 3. Subject Attributes
Information about who is requesting access:
- **role**: Healthcare professional role (CodedValue)
- **provider-identifier**: Provider ID with assigning authority (InstanceIdentifier)
- **provider-institution**: Provider's institution (InstanceIdentifier)
- **consulting-healthcare-facility-type-code**: Type of consulting facility (CodedValue)

#### 4. Environment Attributes
Contextual information:
- **purposeofuse**: Purpose of data access (CodedValue)

## Code Systems

The implementation uses standard HL7 and IHE code systems:

- **BSN (Dutch Citizen Service Number)**: `2.16.840.1.113883.2.4.6.3`
- **Healthcare Facility Types**: `2.16.840.1.113883.2.4.15.1060`
- **Institution IDs**: `2.16.528.1.1007.3.3`
- **Event Codes**: `2.16.840.1.113883.2.4.3.111.5.10.1`
- **Healthcare Professional Roles**: `2.16.840.1.113883.2.4.15.111`
- **Provider IDs**: `2.16.528.1.1007.3.1`
- **Purpose of Use**: `2.16.840.1.113883.1.11.20448`

## Example Output

The generated XML follows the XACML 3.0 SAML profile structure and includes proper namespace declarations for:
- SOAP Envelope (`http://www.w3.org/2003/05/soap-envelope`)
- WS-Addressing (`http://www.w3.org/2005/08/addressing`)
- XACML 3.0 (`urn:oasis:names:tc:xacml:3.0:*`)
- HL7 v3 (`urn:hl7-org:v3`)

See `example.xml` for a complete example of the generated output.
