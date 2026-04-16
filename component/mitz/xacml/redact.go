package xacml

import "regexp"

// BSNRootOID is the HL7 root OID identifying a BSN (Dutch social security number)
// inside an HL7 InstanceIdentifier element.
const BSNRootOID = "2.16.840.1.113883.2.4.6.3"

const redactedExtension = `extension="[REDACTED]"`

// Two patterns cover both attribute orderings XACML emitters may produce.
// They match InstanceIdentifier elements scoped to the BSN root OID and
// rewrite only the `extension` attribute, leaving sibling attributes intact.
// Redaction is keyed on the root OID (not on a known BSN value) so BSNs
// echoed back by MITZ that we never sent ourselves are also scrubbed.
var (
	reBSNExtensionBeforeRoot = regexp.MustCompile(
		`extension="[^"]*"(\s+(?:[A-Za-z:][A-Za-z0-9:]*="[^"]*"\s+)*root="2\.16\.840\.1\.113883\.2\.4\.6\.3")`,
	)
	reBSNRootBeforeExtension = regexp.MustCompile(
		`(root="2\.16\.840\.1\.113883\.2\.4\.6\.3"(?:\s+[A-Za-z:][A-Za-z0-9:]*="[^"]*")*\s+)extension="[^"]*"`,
	)
)

// RedactBSN returns the given XACML payload with every BSN extension value
// stripped from HL7 InstanceIdentifier elements that are scoped to the BSN
// root OID. The result is safe to write to logs.
func RedactBSN(xmlPayload string) string {
	if xmlPayload == "" {
		return ""
	}
	out := reBSNExtensionBeforeRoot.ReplaceAllString(xmlPayload, redactedExtension+`$1`)
	out = reBSNRootBeforeExtension.ReplaceAllString(out, `${1}`+redactedExtension)
	return out
}
