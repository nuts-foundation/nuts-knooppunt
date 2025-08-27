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

func fmtCodable(cc fhir.CodeableConcept) string {
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

func fmtCoding(cd fhir.Coding) string {
	if cd.Display != nil {
		return *cd.Display
	}
	return unknownStr
}

func fmtPeriod(period fhir.Period) string {
	return *period.Start + " - " + *period.End
}

func fmtRef(ref fhir.Reference) string {
	if ref.Display != nil {
		return *ref.Display
	}
	return unknownStr
}

func fmtStatus(status fhir.EndpointStatus) string {
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
		out.PayloadType = fmtCodable(ep.PayloadType[0])
	} else {
		out.PayloadType = unknownStr
	}

	hasPeriod := ep.Period != nil
	if hasPeriod {
		out.Period = fmtPeriod(*ep.Period)
	} else {
		out.Period = unknownStr
	}

	hasManagingOrg := ep.ManagingOrganization != nil
	if hasManagingOrg {
		out.ManagingOrg = fmtRef(*ep.ManagingOrganization)
	} else {
		out.ManagingOrg = unknownStr
	}

	out.ConnectionType = fmtCoding(ep.ConnectionType)
	out.Status = fmtStatus(ep.Status)

	return out
}

func MakeEpListXsProps(eps []fhir.Endpoint) []EpListProps {
	out := make([]EpListProps, len(eps))
	for idx, p := range eps {
		out[idx] = MakeEpListProps(p)
	}
	return out
}

type OrgListProps struct {
	Name   string
	Type   string
	Active string
}

func MakeOrgListProps(org fhir.Organization) (out OrgListProps) {
	if org.Name != nil {
		out.Name = *org.Name
	} else {
		out.Name = unknownStr
	}

	if len(org.Type) > 0 {
		out.Type = fmtCodable(org.Type[0])
	} else {
		out.Type = unknownStr
	}

	if org.Active != nil {
		switch *org.Active {
		case true:
			out.Active = "True"
		case false:
			out.Active = "False"
		}
	} else {
		out.Active = unknownStr
	}

	return out
}

func MakeOrgListXsProps(orgs []fhir.Organization) []OrgListProps {
	out := make([]OrgListProps, len(orgs))
	for idx, op := range orgs {
		out[idx] = MakeOrgListProps(op)
	}
	return out
}
