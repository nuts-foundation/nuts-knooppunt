package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const URANamingSystem = "http://fhir.nl/fhir/NamingSystem/ura"
const UZINamingSystem = "http://fhir.nl/fhir/NamingSystem/uzi"
const KVKNamingSystem = "http://fhir.nl/fhir/NamingSystem/kvk"
const BSNNamingSystem = "http://fhir.nl/fhir/NamingSystem/bsn"
const BSNTransportTokenNamingSystem = "http://fhir.nl/fhir/NamingSystem/bsn-transport-token"
const MCSDPayloadTypeSystem = "http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities"
const MCSDPayloadTypeDirectoryCode = "http://nuts-foundation.github.io/nl-generic-functions-ig/CapabilityStatement/nl-gf-admin-directory-update-client"
const MCSDPayloadTypeConsentNotify = "consent-notify"

var PayloadCoding = fhir.Coding{
	System: to.Ptr(MCSDPayloadTypeSystem),
	Code:   to.Ptr(MCSDPayloadTypeDirectoryCode),
}
