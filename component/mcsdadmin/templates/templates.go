package templates

import (
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed *.html
var tmplFS embed.FS

func RenderWithBase(w http.ResponseWriter, name string, data any) {
	files := []string{
		"base.html",
		name,
	}

	ts, err := template.ParseFS(tmplFS, files...)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

const unknownStr = "N/A"

type EpListProps struct {
	Address        string
	PayloadType    string
	Period         string
	ManagingOrg    string
	ConnectionType string
	Status         string
}

func FmtCodable(cc fhir.CodeableConcept) string {
	if cc.Text != nil {
		return *cc.Text
	}
	if len(cc.Coding) > 0 {
		for _, code := range cc.Coding {
			if code.Display != nil {
				return *code.Display
			}
		}
	}
	return unknownStr
}

func FmtCoding(cd fhir.Coding) string {
	if cd.Display != nil {
		return *cd.Display
	}
	return unknownStr
}

func FmtPeriod(period fhir.Period) string {
	return *period.Start + " - " + *period.End
}

func FmtOrg(org fhir.Organization) string {
	if org.Name != nil {
		return *org.Name
	}
	return unknownStr
}

func FmtRef(ref fhir.Reference) string {
	if ref.Display != nil {
		return *ref.Display
	}
	return unknownStr
}

func FmtStatus(status fhir.EndpointStatus) string {
	switch status {
	case fhir.EndpointStatusActive:
		return "Active"
	case fhir.EndpointStatusSuspended:
		return "Suspended"
	case fhir.EndpointStatusError:
		return "Error"
	case fhir.EndpointStatusOff:
		return "Off"
	case fhir.EndpointStatusEnteredInError:
		return "Error"
	case fhir.EndpointStatusTest:
		return "Test"
	default:
		return unknownStr
	}
}

func MakeEpListProps(ep fhir.Endpoint) (out EpListProps) {
	out.Address = ep.Address

	hasPayload := len(ep.PayloadType) > 0
	if hasPayload {
		out.PayloadType = FmtCodable(ep.PayloadType[0])
	} else {
		out.PayloadType = unknownStr
	}

	hasPeriod := ep.Period != nil
	if hasPeriod {
		out.Period = FmtPeriod(*ep.Period)
	} else {
		out.Period = unknownStr
	}

	hasManagingOrg := ep.ManagingOrganization != nil
	if hasManagingOrg {
		out.ManagingOrg = FmtRef(*ep.ManagingOrganization)
	} else {
		out.ManagingOrg = unknownStr
	}

	out.ConnectionType = FmtCoding(ep.ConnectionType)
	out.Status = FmtStatus(ep.Status)

	return out
}
