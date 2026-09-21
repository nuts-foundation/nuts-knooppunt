package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const URANamingSystem = "http://fhir.nl/fhir/NamingSystem/ura"
const UZINamingSystem = "http://fhir.nl/fhir/NamingSystem/uzi"
const KVKNamingSystem = "http://fhir.nl/fhir/NamingSystem/kvk"
const BSNNamingSystem = "http://fhir.nl/fhir/NamingSystem/bsn"
const BSNTransportTokenNamingSystem = "http://minvws.github.io/generiekefuncties-docs/NamingSystem/nvi-identifier"
const NVICustodianExtensionURL = "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian"
const MCSDPayloadTypeSystem = "http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities"
const MCSDPayloadTypeDirectoryCode = "http://nuts-foundation.github.io/nl-generic-functions-ig/CapabilityStatement/nl-gf-admin-directory-update-client"
const MCSDPayloadTypeConsentNotify = "consent-notify"

// TTA Notifications v0.6 POC (docs/tta-notifications-v06/): payload type codes for
// resolving a partner's notification-delivery and Subscription endpoints via the mCSD
// Query Directory (GF Adressing). No published code system defines these yet — proposed
// by this POC, recorded as a finding under research question 9.
const MCSDPayloadTypeTTANotification = "tta-notification"
const MCSDPayloadTypeTTASubscription = "tta-subscription"

var PayloadCoding = fhir.Coding{
	System: to.Ptr(MCSDPayloadTypeSystem),
	Code:   to.Ptr(MCSDPayloadTypeDirectoryCode),
}
