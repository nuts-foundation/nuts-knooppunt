# XACML Component

This package generates XACML (eXtensible Access Control Markup Language) authorization decision queries for healthcare access control scenarios based on the IHE (Integrating the Healthcare Enterprise) APPC (Access Control) profile.

## Overview

The XACML package creates SOAP envelopes containing XACML authorization decision queries with structured attributes for healthcare access control. Usage examples can be found in the unit tests (see `xacml_test.go`).

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

## Generated Output

The generated XML follows the XACML 3.0 SAML profile structure and includes proper namespace declarations for:
- SOAP Envelope (`http://www.w3.org/2003/05/soap-envelope`)
- WS-Addressing (`http://www.w3.org/2005/08/addressing`)
- XACML 3.0 (`urn:oasis:names:tc:xacml:3.0:*`)
- HL7 v3 (`urn:hl7-org:v3`)

For a concrete example, see `example/example.xml` in this directory.
