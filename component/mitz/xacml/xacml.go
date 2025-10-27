package xacml

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/beevik/etree"
	"github.com/google/uuid"
	dsig "github.com/russellhaering/goxmldsig"
)

// CreateAuthzDecisionQuery generates an unsigned SOAP envelope containing an XACML authorization decision query
// based on the example.xml structure
func CreateAuthzDecisionQuery(req AuthzRequest) (string, error) {
	return createAuthzDecisionQuery(req, nil)
}

// CreateSignedAuthzDecisionQuery generates a signed SOAP envelope containing an XACML authorization decision query
func CreateSignedAuthzDecisionQuery(req AuthzRequest, signingConfig *SigningConfig) (string, error) {
	if signingConfig == nil {
		return "", fmt.Errorf("signing config is required for signed queries")
	}
	return createAuthzDecisionQuery(req, signingConfig)
}

// createAuthzDecisionQuery generates a SOAP envelope (optionally signed) containing an XACML authorization decision query
func createAuthzDecisionQuery(req AuthzRequest, signingConfig *SigningConfig) (string, error) {
	// Hardcoded ToAddress endpoint for MITZ callback
	const toAddress = "http://localhost:8000/4"
	// Create SOAP Envelope
	envelope := etree.NewElement("SOAP-ENV:Envelope")
	envelope.CreateAttr("xmlns:SOAP-ENV", "http://www.w3.org/2003/05/soap-envelope")
	envelope.CreateAttr("xmlns:s", "http://www.w3.org/2001/XMLSchema")
	envelope.CreateAttr("xmlns:types", "urn:oasis:names:tc:xacml:3.0:profile:saml2.0:v2:schema:protocol:wd-14")
	envelope.CreateAttr("xmlns:wsa", "http://www.w3.org/2005/08/addressing")
	envelope.CreateAttr("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance")

	// SOAP Header
	header := envelope.CreateElement("SOAP-ENV:Header")

	action := header.CreateElement("wsa:Action")
	action.SetText("XACMLAuthorizationDecisionQueryRequest")

	messageID := header.CreateElement("wsa:MessageID")
	messageID.SetText("urn:uuid:" + uuid.New().String())

	replyTo := header.CreateElement("wsa:ReplyTo")
	address := replyTo.CreateElement("wsa:Address")
	address.SetText("http://www.w3.org/2005/08/addressing/anonymous")

	to := header.CreateElement("wsa:To")
	to.SetText(toAddress)

	// SOAP Body
	body := envelope.CreateElement("SOAP-ENV:Body")

	xacmlQuery := body.CreateElement("types:XACMLAuthzDecisionQuery")

	request := xacmlQuery.CreateElement("Request")
	request.CreateAttr("CombinedDecision", "false")
	request.CreateAttr("ReturnPolicyIdList", "false")
	request.CreateAttr("xmlns", "urn:oasis:names:tc:xacml:3.0:core:schema:wd-17")

	// Resource Attributes
	resourceAttrs := request.CreateElement("Attributes")
	resourceAttrs.CreateAttr("Category", "urn:oasis:names:tc:xacml:3.0:attribute-category:resource")
	resourceAttrs.CreateAttr("xml:id", "resource")

	// Resource ID (Patient BSN)
	addHL7InstanceIdentifierAttribute(resourceAttrs,
		"urn:oasis:names:tc:xacml:2.0:resource:resource-id",
		"2.16.840.1.113883.2.4.6.3",
		req.PatientBSN)

	// Healthcare Facility Type
	addHL7CodedValueAttribute(resourceAttrs,
		"urn:ihe:iti:appc:2016:document-entry:healthcare-facility-type-code",
		req.HealthcareFacilityType,
		"2.16.840.1.113883.2.4.15.1060")

	// Author Institution ID
	addHL7InstanceIdentifierAttribute(resourceAttrs,
		"urn:ihe:iti:appc:2016:author-institution:id",
		"2.16.528.1.1007.3.3",
		req.AuthorInstitutionID)

	// Action Attributes
	actionAttrs := request.CreateElement("Attributes")
	actionAttrs.CreateAttr("Category", "urn:oasis:names:tc:xacml:3.0:attribute-category:action")
	actionAttrs.CreateAttr("xml:id", "action0")

	// Event Code
	addHL7CodedValueAttribute(actionAttrs,
		"urn:ihe:iti:appc:2016:document-entry:event-code",
		req.EventCode,
		"2.16.840.1.113883.2.4.3.111.5.10.1")

	// Subject Attributes
	subjectAttrs := request.CreateElement("Attributes")
	subjectAttrs.CreateAttr("Category", "urn:oasis:names:tc:xacml:1.0:subject-category:access-subject")
	subjectAttrs.CreateAttr("xml:id", "subject")

	// Subject Role
	addHL7CodedValueAttribute(subjectAttrs,
		"urn:oasis:names:tc:xacml:2.0:subject:role",
		req.SubjectRole,
		"2.16.840.1.113883.2.4.15.111")

	// Provider Identifier
	addHL7InstanceIdentifierAttributeWithAssigningAuthority(subjectAttrs,
		"urn:ihe:iti:xua:2017:subject:provider-identifier",
		"2.16.528.1.1007.3.1",
		req.ProviderID,
		"CIBG")

	// Provider Institution
	addHL7InstanceIdentifierAttribute(subjectAttrs,
		"urn:nl:otv:names:tc:1.0:subject:provider-institution",
		"2.16.528.1.1007.3.3",
		req.ProviderInstitutionID)

	// Consulting Healthcare Facility Type
	addHL7CodedValueAttribute(subjectAttrs,
		"urn:nl:otv:names:tc:1.0:subject:consulting-healthcare-facility-type-code",
		req.ConsultingFacilityType,
		"2.16.840.1.113883.2.4.15.1060")

	// Environment Attributes
	envAttrs := request.CreateElement("Attributes")
	envAttrs.CreateAttr("Category", "urn:oasis:names:tc:xacml:3.0:attribute-category:environment")
	envAttrs.CreateAttr("xml:id", "environment")

	// Purpose of Use
	addHL7CodedValueAttribute(envAttrs,
		"urn:oasis:names:tc:xspa:1.0:subject:purposeofuse",
		req.PurposeOfUse,
		"2.16.840.1.113883.1.11.20448")

	// Sign the XACML query if signing config is provided
	if signingConfig != nil {
		err := signXACMLQuery(xacmlQuery, signingConfig)
		if err != nil {
			return "", fmt.Errorf("failed to sign XACML query: %w", err)
		}
	}

	// Convert to string
	doc := etree.NewDocument()
	doc.SetRoot(envelope)
	doc.Indent(4)

	xmlString, err := doc.WriteToString()
	if err != nil {
		return "", fmt.Errorf("failed to generate XACML query: %w", err)
	}

	return xmlString, nil
}

// addHL7InstanceIdentifierAttribute adds an Attribute with HL7 InstanceIdentifier as AttributeValue
func addHL7InstanceIdentifierAttribute(parent *etree.Element, attributeID, root, extension string) {
	attr := parent.CreateElement("Attribute")
	attr.CreateAttr("AttributeId", attributeID)
	attr.CreateAttr("IncludeInResult", "true")

	attrValue := attr.CreateElement("AttributeValue")
	attrValue.CreateAttr("DataType", "urn:hl7-org:v3#II")

	instanceID := attrValue.CreateElement("InstanceIdentifier")
	instanceID.CreateAttr("extension", extension)
	instanceID.CreateAttr("root", root)
	instanceID.CreateAttr("xmlns", "urn:hl7-org:v3")
}

// addHL7InstanceIdentifierAttributeWithAssigningAuthority adds an InstanceIdentifier with assigningAuthorityName
func addHL7InstanceIdentifierAttributeWithAssigningAuthority(parent *etree.Element, attributeID, root, extension, assigningAuthority string) {
	attr := parent.CreateElement("Attribute")
	attr.CreateAttr("AttributeId", attributeID)
	attr.CreateAttr("IncludeInResult", "true")

	attrValue := attr.CreateElement("AttributeValue")
	attrValue.CreateAttr("DataType", "urn:hl7-org:v3#II")

	instanceID := attrValue.CreateElement("InstanceIdentifier")
	instanceID.CreateAttr("assigningAuthorityName", assigningAuthority)
	instanceID.CreateAttr("extension", extension)
	instanceID.CreateAttr("root", root)
	instanceID.CreateAttr("xmlns", "urn:hl7-org:v3")
}

// addHL7CodedValueAttribute adds an Attribute with HL7 CodedValue as AttributeValue
func addHL7CodedValueAttribute(parent *etree.Element, attributeID, code, codeSystem string) {
	attr := parent.CreateElement("Attribute")
	attr.CreateAttr("AttributeId", attributeID)
	attr.CreateAttr("IncludeInResult", "true")

	attrValue := attr.CreateElement("AttributeValue")
	attrValue.CreateAttr("DataType", "urn:hl7-org:v3#CV")

	codedValue := attrValue.CreateElement("CodedValue")
	codedValue.CreateAttr("code", code)
	codedValue.CreateAttr("codeSystem", codeSystem)
	codedValue.CreateAttr("xmlns", "urn:hl7-org:v3")
}

// signXACMLQuery signs the XACML query element using XML digital signature
func signXACMLQuery(xacmlQuery *etree.Element, config *SigningConfig) error {
	// Get the Request element - this is what we'll sign
	request := xacmlQuery.SelectElement("Request")
	if request == nil {
		return fmt.Errorf("Request element not found in XACML query")
	}

	// Add an ID to the Request element for referencing in signature
	queryID := "_" + uuid.New().String()
	request.CreateAttr("xml:id", queryID)

	// Step 1: Canonicalize the Request element
	canonicalizer := dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	canonicalRequest, err := canonicalizer.Canonicalize(request)
	if err != nil {
		return fmt.Errorf("failed to canonicalize request: %w", err)
	}

	// Step 2: Compute the digest
	digest := sha256.Sum256(canonicalRequest)
	digestValue := base64.StdEncoding.EncodeToString(digest[:])

	// Step 3: Build SignedInfo
	signedInfo := buildXACMLSignedInfo(queryID, digestValue)

	// Canonicalize SignedInfo
	canonicalSignedInfo, err := canonicalizer.Canonicalize(signedInfo)
	if err != nil {
		return fmt.Errorf("failed to canonicalize SignedInfo: %w", err)
	}

	// Step 4: Sign the canonicalized SignedInfo
	hash := sha256.Sum256(canonicalSignedInfo)
	signature, err := config.PrivateKey.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return fmt.Errorf("failed to create signature: %w", err)
	}
	signatureValue := base64.StdEncoding.EncodeToString(signature)

	// Step 5: Build the Signature element
	signatureElement := etree.NewElement("ds:Signature")
	signatureElement.CreateAttr("xmlns:ds", "http://www.w3.org/2000/09/xmldsig#")
	signatureElement.AddChild(signedInfo)

	sigValue := signatureElement.CreateElement("ds:SignatureValue")
	sigValue.SetText(signatureValue)

	keyInfo := signatureElement.CreateElement("ds:KeyInfo")
	x509Data := keyInfo.CreateElement("ds:X509Data")
	x509Cert := x509Data.CreateElement("ds:X509Certificate")

	// Encode the certificate chain
	combinedCertBytes := make([]byte, 0)
	for _, certBytes := range config.Certificate {
		combinedCertBytes = append(combinedCertBytes, certBytes...)
	}
	x509Cert.SetText(base64.StdEncoding.EncodeToString(combinedCertBytes))

	// Step 6: Insert the Signature as the first child of XACMLAuthzDecisionQuery
	xacmlQuery.InsertChildAt(0, signatureElement)

	return nil
}

// buildXACMLSignedInfo creates the SignedInfo element for XACML query signing
func buildXACMLSignedInfo(requestID, digestValue string) *etree.Element {
	signedInfo := etree.NewElement("ds:SignedInfo")
	signedInfo.CreateAttr("xmlns:ds", "http://www.w3.org/2000/09/xmldsig#")

	canonicalizationMethod := signedInfo.CreateElement("ds:CanonicalizationMethod")
	canonicalizationMethod.CreateAttr("Algorithm", "http://www.w3.org/2001/10/xml-exc-c14n#")

	signatureMethod := signedInfo.CreateElement("ds:SignatureMethod")
	signatureMethod.CreateAttr("Algorithm", "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256")

	reference := signedInfo.CreateElement("ds:Reference")
	reference.CreateAttr("URI", "#"+requestID)

	transforms := reference.CreateElement("ds:Transforms")
	transform1 := transforms.CreateElement("ds:Transform")
	transform1.CreateAttr("Algorithm", "http://www.w3.org/2000/09/xmldsig#enveloped-signature")
	transform2 := transforms.CreateElement("ds:Transform")
	transform2.CreateAttr("Algorithm", "http://www.w3.org/2001/10/xml-exc-c14n#")

	digestMethod := reference.CreateElement("ds:DigestMethod")
	digestMethod.CreateAttr("Algorithm", "http://www.w3.org/2001/04/xmlenc#sha256")

	digestValueElement := reference.CreateElement("ds:DigestValue")
	digestValueElement.SetText(digestValue)

	return signedInfo
}

// ParseXACMLResponse parses the XACML authorization decision query response and extracts the decision.
// The response is expected to be a SOAP envelope containing an XACML Response with a Result/Decision element.
// Example structure:
//
//	<s:Envelope>
//	  <s:Body>
//	    <Response>
//	      <Result>
//	        <Decision>Permit</Decision>
//	        ...
//	      </Result>
//	    </Response>
//	  </s:Body>
//	</s:Envelope>
func ParseXACMLResponse(responseXML []byte) (*XACMLResponse, error) {
	doc := etree.NewDocument()
	err := doc.ReadFromBytes(responseXML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XACML response XML: %w", err)
	}

	root := doc.Root()
	if root == nil {
		return nil, fmt.Errorf("invalid XACML response: no root element found")
	}

	// Navigate to the Decision element: /Envelope/Body/Response/Result/Decision
	// Using etree's FindElement with paths that handle namespaces
	var decisionText string

	// Try to find Response element with namespace
	responseElem := root.FindElement(".//{urn:oasis:names:tc:xacml:3.0:core:schema:wd-17}Response")
	if responseElem == nil {
		// Try without namespace
		responseElem = root.FindElement(".//Response")
	}
	if responseElem == nil {
		return nil, fmt.Errorf("invalid XACML response: Response element not found")
	}

	// Find Result element
	resultElem := responseElem.FindElement("{urn:oasis:names:tc:xacml:3.0:core:schema:wd-17}Result")
	if resultElem == nil {
		// Try without namespace
		resultElem = responseElem.FindElement("Result")
	}
	if resultElem == nil {
		return nil, fmt.Errorf("invalid XACML response: Result element not found")
	}

	// Find Decision element
	decisionElem := resultElem.FindElement("{urn:oasis:names:tc:xacml:3.0:core:schema:wd-17}Decision")
	if decisionElem == nil {
		// Try without namespace
		decisionElem = resultElem.FindElement("Decision")
	}
	if decisionElem == nil {
		return nil, fmt.Errorf("invalid XACML response: Decision element not found")
	}

	decisionText = decisionElem.Text()
	if decisionText == "" {
		return nil, fmt.Errorf("invalid XACML response: Decision element is empty")
	}

	// Convert decision text to Decision constant
	var decision Decision
	switch decisionText {
	case "Permit":
		decision = DecisionPermit
	case "Deny":
		decision = DecisionDeny
	case "NotApplicable":
		decision = DecisionNotApplicable
	case "Indeterminate":
		decision = DecisionIndeterminate
	default:
		return nil, fmt.Errorf("invalid decision value: %s", decisionText)
	}

	return &XACMLResponse{
		Decision: decision,
		RawXML:   responseXML,
	}, nil
}
