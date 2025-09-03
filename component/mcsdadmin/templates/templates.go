package templates

import (
	"embed"
	"html/template"
	"io"

	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed *.html
var tmplFS embed.FS

func RenderWithBase(w io.Writer, name string, data any) {
	files := []string{
		"base.html",
		name,
	}

	ts, err := template.ParseFS(tmplFS, files...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse template")
		return
	}

	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to execute template")
		return
	}
}

const unknownStr = "N/A"

type EpListProps struct {
	Id             string
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

func MakeEpListProps(ep fhir.Endpoint) (out EpListProps) {
	if ep.Id != nil {
		out.Id = *ep.Id
	}

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
	out.Status = ep.Status.Display()

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
	Id     string
	Name   string
	Type   string
	Active bool
}

func MakeOrgListProps(org fhir.Organization) (out OrgListProps) {
	if org.Id != nil {
		out.Id = *org.Id
	}

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
		if *org.Active {
			out.Active = true
		}
	} else {
		out.Active = false
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

type ServiceListProps struct {
	Id         string
	Name       string
	Type       string
	Active     bool
	ProvidedBy string
}

func MakeServiceListProps(service fhir.HealthcareService) (out ServiceListProps) {
	if service.Id != nil {
		out.Id = *service.Id
	}

	if service.Name != nil {
		out.Name = *service.Name
	} else {
		out.Name = unknownStr
	}

	if len(service.Type) > 0 {
		out.Type = fmtCodable(service.Type[0])
	} else {
		out.Type = unknownStr
	}

	if service.Active != nil {
		if *service.Active {
			out.Active = true
		}
	} else {
		out.Active = false
	}

	if service.ProvidedBy != nil {
		ref := *service.ProvidedBy
		if ref.Display != nil {
			out.ProvidedBy = *ref.Display
		} else {
			out.ProvidedBy = unknownStr
		}
	} else {
		out.ProvidedBy = unknownStr
	}

	return out
}

func MakeServiceListXsProps(services []fhir.HealthcareService) []ServiceListProps {
	out := make([]ServiceListProps, len(services))
	for idx, ser := range services {
		out[idx] = MakeServiceListProps(ser)
	}
	return out
}

type LocationListProps struct {
	Id           string
	Name         string
	Type         string
	Status       string
	PhysicalType string
}

func MakeLocationListProps(location fhir.Location) (out LocationListProps) {
	if location.Id != nil {
		out.Id = *location.Id
	}

	if location.Name != nil {
		out.Name = *location.Name
	} else {
		out.Name = unknownStr
	}

	if len(location.Type) > 0 {
		out.Type = fmtCodable(location.Type[0])
	} else {
		out.Type = unknownStr
	}

	if location.Status != nil {
		status := *location.Status
		out.Status = status.Display()
	} else {
		out.Status = unknownStr
	}

	if location.PhysicalType != nil {
		out.PhysicalType = fmtCodable(*location.PhysicalType)
	} else {
		out.PhysicalType = unknownStr
	}

	return out
}

func MakeLocationListXsProps(locations []fhir.Location) []LocationListProps {
	out := make([]LocationListProps, len(locations))
	for idx, l := range locations {
		out[idx] = MakeLocationListProps(l)
	}
	return out
}
