package templates

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"log/slog"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed *.html
var tmplFS embed.FS

var partialTemplates = []string{}

func init() {
	files, err := tmplFS.ReadDir(".")
	if err != nil {
		slog.Error("could not initiate template files", logging.Error(err))
	}

	for _, file := range files {
		name := file.Name()
		startsWithUnderscore := name[:1] == "_"
		if startsWithUnderscore {
			partialTemplates = append(partialTemplates, name)
		}
	}
}

func RenderWithBase(w io.Writer, name string, data any) {
	files := []string{
		"base.html",
		name,
	}
	files = append(files, partialTemplates...)

	// Create template with custom functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
		"iterate": func(count int) []int {
			var i int
			var items []int
			for i = 0; i < count; i++ {
				items = append(items, i)
			}
			return items
		},
	}

	ts, err := template.New("base").Funcs(funcMap).ParseFS(tmplFS, files...)
	if err != nil {
		slog.Error("Failed to parse template", logging.Error(err))
		return
	}

	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		slog.Error("Failed to execute template", logging.Error(err))
		return
	}
}

func RenderPartial(w io.Writer, name string, data any) {
	filename := fmt.Sprintf("%s.html", name)
	ts, err := template.ParseFS(tmplFS, filename)
	if err != nil {
		slog.Error("Failed to parse template", logging.Error(err))
		return
	}

	err = ts.ExecuteTemplate(w, name, data)
	if err != nil {
		slog.Error("Failed to execute template", logging.Error(err))
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
	Id            string
	Name          string
	URA           string
	EndpointCount string
	Type          string
	Active        bool
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

	for _, idn := range org.Identifier {
		if idn.System != nil && idn.Value != nil {
			if *idn.System == coding.URANamingSystem {
				out.URA = *idn.Value
			}
		}
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

	epCount := len(org.Endpoint)
	out.EndpointCount = fmt.Sprint(epCount)

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
	Id            string
	Name          string
	Type          string
	Active        bool
	ProvidedBy    string
	EndpointCount string
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

	epCount := len(service.Endpoint)
	out.EndpointCount = fmt.Sprint(epCount)

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

type EndpointCardProps struct {
	Endpoint     fhir.Endpoint
	Organization fhir.Organization
}

func MakeEndpointCards(endpoints []fhir.Endpoint, org fhir.Organization) []EndpointCardProps {
	cards := make([]EndpointCardProps, len(endpoints))
	for i, endp := range endpoints {
		cards[i] = EndpointCardProps{
			Endpoint:     endp,
			Organization: org,
		}
	}
	return cards
}

type HealthcareServiceEndpointCardProps struct {
	Endpoint          fhir.Endpoint
	HealthcareService fhir.HealthcareService
}

func MakeHealthcareServiceEndpointCards(endpoints []fhir.Endpoint, service fhir.HealthcareService) []HealthcareServiceEndpointCardProps {
	cards := make([]HealthcareServiceEndpointCardProps, len(endpoints))
	for i, endp := range endpoints {
		cards[i] = HealthcareServiceEndpointCardProps{
			Endpoint:          endp,
			HealthcareService: service,
		}
	}
	return cards
}

type PractitionerRoleProps struct {
	Id           string
	Uzi          string
	Organization string
	Code         string
	Telecom      string
}

func MakePractitionerRoleProps(role fhir.PractitionerRole) PractitionerRoleProps {
	out := PractitionerRoleProps{}
	if role.Id != nil {
		out.Id = *role.Id
	} else {
		out.Id = unknownStr
	}

	ref := role.Practitioner
	if ref != nil && ref.Identifier != nil && ref.Identifier.Value != nil {
		out.Uzi = *ref.Identifier.Value
	} else {
		out.Uzi = unknownStr
	}

	if role.Organization != nil {
		out.Organization = fmtRef(*role.Organization)
	} else {
		out.Organization = unknownStr
	}

	if len(role.Code) > 0 {
		out.Code = fmtCodable(role.Code[0])
	} else {
		out.Code = unknownStr
	}

	out.Telecom = unknownStr

	return out
}

func MakePractitionerRoleXsProps(roles []fhir.PractitionerRole) []PractitionerRoleProps {
	out := make([]PractitionerRoleProps, len(roles))
	for idx, role := range roles {
		out[idx] = MakePractitionerRoleProps(role)
	}
	return out
}
