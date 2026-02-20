package mcsdadmin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	formdata "github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/formdata"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/static"
	tmpls "github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/templates"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/nuts-foundation/nuts-knooppunt/lib/profile"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Config struct {
	FHIRBaseURL string `koanf:"fhirbaseurl"`
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	config     Config
	fhirClient fhirclient.Client
}

var client fhirclient.Client

func New(config Config) *Component {
	baseURL, err := url.Parse(config.FHIRBaseURL)
	if err != nil {
		slog.Error("Failed to start MCSD admin component, invalid FHIRBaseURL", logging.Error(err))
		return nil
	}

	client = fhirclient.New(baseURL, tracing.NewHTTPClient(), fhirutil.ClientConfig())

	return &Component{
		config:     config,
		fhirClient: client,
	}
}

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(_ context.Context) error {
	// Nothing to do
	return nil
}

// Route handling

var fileServer = http.FileServer(http.FS(static.FS))

func (c Component) RegisterHttpHandlers(mux *http.ServeMux, _ *http.ServeMux) {
	// Static file serving for CSS and fonts
	mux.Handle("GET /mcsdadmin/css/", http.StripPrefix("/mcsdadmin/", fileServer))
	mux.Handle("GET /mcsdadmin/js/", http.StripPrefix("/mcsdadmin/", fileServer))
	mux.Handle("GET /mcsdadmin/webfonts/", http.StripPrefix("/mcsdadmin/", fileServer))

	mux.HandleFunc("GET /mcsdadmin/healthcareservice", listServices)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/new", newService)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/new", newServicePost)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/{id}/endpoints", associateHealthcareServiceEndpoints)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/{id}/endpoints", associateHealthcareServiceEndpointsPost)
	mux.HandleFunc("DELETE /mcsdadmin/healthcareservice/{id}/endpoints", associateHealthcareServiceEndpointsDelete)
	mux.HandleFunc("GET /mcsdadmin/organization", listOrganizations)
	mux.HandleFunc("GET /mcsdadmin/organization/new", newOrganization)
	mux.HandleFunc("POST /mcsdadmin/organization/new", newOrganizationPost)
	mux.HandleFunc("GET /mcsdadmin/organization/{id}/endpoints", associateEndpoints)
	mux.HandleFunc("POST /mcsdadmin/organization/{id}/endpoints", associateEndpointsPost)
	mux.HandleFunc("DELETE /mcsdadmin/organization/{id}/endpoints", associateEndpointsDelete)
	mux.HandleFunc("GET /mcsdadmin/endpoint", listEndpoints)
	mux.HandleFunc("GET /mcsdadmin/endpoint/new", newEndpoint)
	mux.HandleFunc("POST /mcsdadmin/endpoint/new", newEndpointPost)
	mux.HandleFunc("GET /mcsdadmin/location", listLocations)
	mux.HandleFunc("GET /mcsdadmin/location/new", newLocation)
	mux.HandleFunc("POST /mcsdadmin/location/new", newLocationPost)
	mux.HandleFunc("DELETE /mcsdadmin/endpoint/{id}", deleteHandler("Endpoint"))
	mux.HandleFunc("DELETE /mcsdadmin/location/{id}", deleteHandler("Location"))
	mux.HandleFunc("DELETE /mcsdadmin/healthcareservice/{id}", deleteHandler("HealthcareService"))
	mux.HandleFunc("DELETE /mcsdadmin/organization/{id}", deleteHandler("Organization"))
	mux.HandleFunc("GET /mcsdadmin/practitionerrole", listPractitionerRole)
	mux.HandleFunc("GET /mcsdadmin/practitionerrole/new", newPractitionerRole)
	mux.HandleFunc("POST /mcsdadmin/practitionerrole/new", newPractitionerRolePost)
	mux.HandleFunc("GET /mcsdadmin", homePage)
	mux.HandleFunc("GET /mcsdadmin/", notFound)
}

func listServices(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderPaginatedList[fhir.HealthcareService, tmpls.ServiceListProps](client, w, r, tmpls.MakeServiceListXsProps)
}

func newService(w http.ResponseWriter, r *http.Request) {
	organizations, err := findAll[fhir.Organization](client)
	if err != nil {
		internalError(w, r, "could not load organizations", err)
		return
	}

	props := struct {
		Types         []fhir.Coding
		Organizations []fhir.Organization
	}{
		Organizations: organizations,
		Types:         valuesets.ServiceTypeCodings,
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "healthcareservice_edit.html", props)
}

func newServicePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		badRequest(w, r, "invalid form input", err)
		return
	}

	service := fhir.HealthcareService{
		Meta: &fhir.Meta{
			Profile: []string{profile.NLGenericFunctionHealthcareService},
		},
	}
	name := r.PostForm.Get("name")
	service.Name = &name
	active := r.PostForm.Get("active") == "true"
	service.Active = &active

	codables, ok := formdata.CodablesFromForm(r.PostForm, valuesets.ServiceTypeCodings, "type")
	if !ok {
		badRequest(w, r, "Could not find type all type codes")
		return
	}
	service.Type = codables

	reference := "Organization/" + r.PostForm.Get("providedById")
	service.ProvidedBy = &fhir.Reference{
		Reference: &reference,
		Type:      to.Ptr("Organization"),
	}

	var providedByOrg fhir.Organization
	err = client.Read(reference, &providedByOrg)
	if err != nil {
		badRequest(w, r, "failed to find referred organisation", err)
		return
	}
	service.ProvidedBy.Display = providedByOrg.Name

	var resSer fhir.HealthcareService
	err = client.Create(service, &resSer)
	if err != nil {
		internalError(w, r, "could not create FHIR resource", err)
		return
	}

	w.WriteHeader(http.StatusCreated)

	renderList[fhir.HealthcareService, tmpls.ServiceListProps](client, w, tmpls.MakeServiceListXsProps)
}

func listOrganizations(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderPaginatedList[fhir.Organization, tmpls.OrgListProps](client, w, r, tmpls.MakeOrgListXsProps)
}

func newOrganization(w http.ResponseWriter, r *http.Request) {
	organizations, err := findAll[fhir.Organization](client)
	if err != nil {
		internalError(w, r, "could not load organizations", err)
		return
	}
	orgsExists := len(organizations) > 0

	w.WriteHeader(http.StatusOK)

	props := struct {
		Types         []fhir.Coding
		Organizations []fhir.Organization
		OrgsExist     bool
	}{
		Types:         valuesets.OrganizationTypeCodings,
		Organizations: organizations,
		OrgsExist:     orgsExists,
	}

	tmpls.RenderWithBase(w, "organization_edit.html", props)
}

func newOrganizationPost(w http.ResponseWriter, r *http.Request) {
	slog.DebugContext(r.Context(), "New post for organization resource")

	err := r.ParseForm()
	if err != nil {
		badRequest(w, r, "invalid form input", err)
		return
	}

	org := fhir.Organization{
		Meta: &fhir.Meta{
			Profile: []string{profile.NLGenericFunctionOrganization},
		},
	}
	name := r.PostForm.Get("name")
	org.Name = &name
	uraString := r.PostForm.Get("identifier")
	partOf := r.PostForm.Get("part-of")

	// Validate: organization must have either URA identifier or partOf reference
	if uraString == "" && partOf == "" {
		badRequest(w, r, "organization must have either a URA identifier or a parent organization (part-of)")
		return
	}

	// Set identifier if provided
	if uraString != "" {
		org.Identifier = []fhir.Identifier{
			uraIdentifier(uraString),
		}
	}

	codables, ok := formdata.CodablesFromForm(r.PostForm, valuesets.OrganizationTypeCodings, "type")
	if !ok {
		badRequest(w, r, "could not find all type codes")
		return
	}
	org.Type = codables

	active := r.PostForm.Get("active") == "true"
	org.Active = &active

	if len(partOf) > 0 {
		reference := "Organization/" + partOf
		org.PartOf = &fhir.Reference{
			Reference: &reference,
			Type:      to.Ptr("Organization"),
		}
		var parentOrg fhir.Organization
		err = client.Read(reference, &parentOrg)
		if err != nil {
			internalError(w, r, "could not find organization", err)
			return
		}
		org.PartOf.Display = parentOrg.Name
	}

	var resOrg fhir.Organization
	err = client.Create(org, &resOrg)
	if err != nil {
		internalError(w, r, "could not create FHIR resource", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	renderList[fhir.Organization, tmpls.OrgListProps](client, w, tmpls.MakeOrgListXsProps)
}

func associateEndpoints(w http.ResponseWriter, req *http.Request) {
	orgId := req.PathValue("id")
	path := fmt.Sprintf("Organization/%s", orgId)
	var org fhir.Organization
	err := client.Read(path, &org)
	if err != nil {
		internalError(w, req, "could not read organization resource", err)
		return
	}

	endpoints := make([]fhir.Endpoint, 0, len(org.Endpoint))
	for _, ref := range org.Endpoint {
		var ep fhir.Endpoint
		if ref.Reference == nil {
			continue
		}
		err := client.Read(*ref.Reference, &ep)
		if err != nil {
			internalError(w, req, "could not read referenced resource", err)
			return
		}
		endpoints = append(endpoints, ep)
	}

	allEndpoints, err := findAll[fhir.Endpoint](client)
	if err != nil {
		internalError(w, req, "could not load endpoints", err)
		return
	}

	props := struct {
		Organization  fhir.Organization
		EndpointCards []tmpls.EndpointCardProps
		AllEndpoints  []fhir.Endpoint
	}{
		Organization:  org,
		EndpointCards: tmpls.MakeEndpointCards(endpoints, org),
		AllEndpoints:  allEndpoints,
	}
	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "organization_endpoints.html", props)
}

func associateHealthcareServiceEndpoints(w http.ResponseWriter, req *http.Request) {
	serviceId := req.PathValue("id")
	path := fmt.Sprintf("HealthcareService/%s", serviceId)
	var service fhir.HealthcareService
	err := client.Read(path, &service)
	if err != nil {
		internalError(w, req, "could not read healthcare service resource", err)
		return
	}

	endpoints := make([]fhir.Endpoint, 0, len(service.Endpoint))
	for _, ref := range service.Endpoint {
		var ep fhir.Endpoint
		if ref.Reference == nil {
			continue
		}
		err := client.Read(*ref.Reference, &ep)
		if err != nil {
			internalError(w, req, "could not read referenced resource", err)
			return
		}
		endpoints = append(endpoints, ep)
	}

	allEndpoints, err := findAll[fhir.Endpoint](client)
	if err != nil {
		internalError(w, req, "could not load endpoints", err)
		return
	}

	props := struct {
		HealthcareService fhir.HealthcareService
		EndpointCards     []tmpls.HealthcareServiceEndpointCardProps
		AllEndpoints      []fhir.Endpoint
	}{
		HealthcareService: service,
		EndpointCards:     tmpls.MakeHealthcareServiceEndpointCards(endpoints, service),
		AllEndpoints:      allEndpoints,
	}
	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "healthcareservice_endpoints.html", props)
}

func associateEndpointsPost(w http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		badRequest(w, req, "invalid form input", err)
		return
	}

	selectedId := req.PostForm.Get("selected-endpoint")
	selected, err := findById[fhir.Endpoint](selectedId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orgId := req.PathValue("id")
	organization, err := findById[fhir.Organization](orgId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	foundIdx := slices.IndexFunc(organization.Endpoint, func(ref fhir.Reference) bool {
		epId := idFromRef(ref)
		return epId == selectedId
	})
	if foundIdx > -1 {
		http.Error(w, "endpoint already associated with organization", http.StatusBadRequest)
		return
	}

	selectedPath := fmt.Sprintf("Endpoint/%s", selectedId)
	ref := fhir.Reference{
		Reference: &selectedPath,
	}
	organization.Endpoint = append(organization.Endpoint, ref)

	orgPath := fmt.Sprintf("Organization/%s", orgId)
	var resultOrg fhir.Organization
	err = client.Update(orgPath, organization, &resultOrg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	props := tmpls.EndpointCardProps{
		Endpoint:     selected,
		Organization: resultOrg,
	}
	tmpls.RenderPartial(w, "_card_endpoint", props)
}

func associateHealthcareServiceEndpointsPost(w http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		badRequest(w, req, "invalid form input", err)
		return
	}

	selectedId := req.PostForm.Get("selected-endpoint")
	selected, err := findById[fhir.Endpoint](selectedId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	serviceId := req.PathValue("id")
	service, err := findById[fhir.HealthcareService](serviceId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	foundIdx := slices.IndexFunc(service.Endpoint, func(ref fhir.Reference) bool {
		epId := idFromRef(ref)
		return epId == selectedId
	})
	if foundIdx > -1 {
		http.Error(w, "endpoint already associated with healthcare service", http.StatusBadRequest)
		return
	}

	selectedPath := fmt.Sprintf("Endpoint/%s", selectedId)
	ref := fhir.Reference{
		Reference: &selectedPath,
	}
	service.Endpoint = append(service.Endpoint, ref)

	servicePath := fmt.Sprintf("HealthcareService/%s", serviceId)
	var resultService fhir.HealthcareService
	err = client.Update(servicePath, service, &resultService)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	props := tmpls.HealthcareServiceEndpointCardProps{
		Endpoint:          selected,
		HealthcareService: resultService,
	}
	tmpls.RenderPartial(w, "_card_endpoint_healthcareservice", props)
}

func associateEndpointsDelete(w http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		badRequest(w, req, "invalid form input", err)
		return
	}

	orgId := req.PathValue("id")
	organization, err := findById[fhir.Organization](orgId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	epId := req.URL.Query().Get("endpointId")
	epFound := false
	for i, ref := range organization.Endpoint {
		refId := idFromRef(ref)
		if refId == epId {
			organization.Endpoint = slices.Delete(organization.Endpoint, i, i+1)
			epFound = true
		}
	}
	if !epFound {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orgPath := fmt.Sprintf("Organization/%s", orgId)
	var orgResult fhir.Organization
	err = client.Update(orgPath, organization, &orgResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func associateHealthcareServiceEndpointsDelete(w http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		badRequest(w, req, "invalid form input", err)
		return
	}

	serviceId := req.PathValue("id")
	service, err := findById[fhir.HealthcareService](serviceId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	epId := req.URL.Query().Get("endpointId")
	epFound := false
	for i, ref := range service.Endpoint {
		refId := idFromRef(ref)
		if refId == epId {
			service.Endpoint = slices.Delete(service.Endpoint, i, i+1)
			epFound = true
		}
	}
	if !epFound {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	servicePath := fmt.Sprintf("HealthcareService/%s", serviceId)
	var serviceResult fhir.HealthcareService
	err = client.Update(servicePath, service, &serviceResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func listEndpoints(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderPaginatedList[fhir.Endpoint, tmpls.EpListProps](client, w, r, tmpls.MakeEpListXsProps)
}

func newEndpoint(w http.ResponseWriter, _ *http.Request) {
	organizations, err := findAll[fhir.Organization](client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	healthcareServices, err := findAll[fhir.HealthcareService](client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		ConnectionTypes    []fhir.Coding
		Organizations      []fhir.Organization
		HealthcareServices []fhir.HealthcareService
		PayloadTypes       []fhir.Coding
		PurposeOfUse       []fhir.Coding
		Status             []fhir.Coding
	}{
		ConnectionTypes:    valuesets.EndpointConnectionTypeCodings,
		Organizations:      organizations,
		HealthcareServices: healthcareServices,
		PayloadTypes:       valuesets.EndpointPayloadTypeCodings,
		PurposeOfUse:       valuesets.PurposeOfUseCodings,
		Status:             valuesets.EndpointStatusCodings,
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "endpoint_edit.html", props)
}

func newEndpointPost(w http.ResponseWriter, r *http.Request) {
	slog.DebugContext(r.Context(), "New post for Endpoint resource")

	err := r.ParseForm()
	if err != nil {
		badRequest(w, r, "invalid form input", err)
		return
	}

	endpoint := fhir.Endpoint{
		Meta: &fhir.Meta{
			Profile: []string{profile.NLGenericFunctionEndpoint},
		},
	}
	address := r.PostForm.Get("address")
	if address == "" {
		http.Error(w, "bad request: missing address", http.StatusBadRequest)
		return
	}
	endpoint.Address = address

	codables, ok := formdata.CodablesFromFormWithCustom(r.PostForm, valuesets.EndpointPayloadTypeCodings, "payload-type")
	if !ok {
		badRequest(w, r, "could not find all type codes")
		return
	}
	if len(codables) < 1 {
		badRequest(w, r, "missing payload type")
		return
	}
	endpoint.PayloadType = codables

	periodStart := r.PostForm.Get("period-start")
	periodEnd := r.PostForm.Get("period-end")
	if (len(periodStart) > 0) && (len(periodEnd) > 0) {
		endpoint.Period = &fhir.Period{
			Start: &periodStart,
			End:   &periodEnd,
		}
	}

	contactValue := r.PostForm.Get("contact")
	if len(contactValue) > 0 {
		contact := fhir.ContactPoint{
			Value: &contactValue,
		}
		endpoint.Contact = []fhir.ContactPoint{contact}
	}

	kvkStr := r.PostForm.Get("managing-org")
	if len(kvkStr) > 0 {
		ref := fhir.Reference{
			Identifier: to.Ptr(fhir.Identifier{
				System: to.Ptr(coding.KVKNamingSystem),
				Value:  to.Ptr(kvkStr),
			}),
		}
		endpoint.ManagingOrganization = to.Ptr(ref)
	}

	var connectionType fhir.Coding
	connectionTypeId := r.PostForm.Get("connection-type")
	connectionType, ok = valuesets.CodingFrom(valuesets.EndpointConnectionTypeCodings, connectionTypeId)
	if ok {
		endpoint.ConnectionType = connectionType
	} else {
		http.Error(w, "bad request: missing connection type", http.StatusBadRequest)
		return
	}

	purposeOfUseId := r.PostForm.Get("purpose-of-use")
	purposeOfUse, ok := valuesets.CodableFrom(valuesets.PurposeOfUseCodings, purposeOfUseId)
	if ok {
		extension := fhir.Extension{
			Url:                  "https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.PurposeOfUse",
			ValueCodeableConcept: &purposeOfUse,
		}
		endpoint.Extension = append(endpoint.Extension, extension)
	}

	status := r.PostForm.Get("status")
	endpoint.Status, ok = valuesets.EndpointStatusFrom(status)
	if !ok {
		http.Error(w, "bad request: missing status", http.StatusBadRequest)
		return
	}

	var resEp fhir.Endpoint
	err = client.Create(endpoint, &resEp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var epRef fhir.Reference
	epRef.Type = to.Ptr("Endpoint")
	epRef.Reference = to.Ptr("Endpoint/" + *resEp.Id)

	forResourceStr := r.PostForm.Get("endpoint-for")
	if len(forResourceStr) > 0 {
		// The value now contains the resource type prefix (e.g., "Organization/123" or "HealthcareService/456")
		if strings.HasPrefix(forResourceStr, "Organization/") {
			var owningOrg fhir.Organization
			err = client.Read(forResourceStr, &owningOrg)
			if err != nil {
				http.Error(w, "bad request: could not find organization", http.StatusBadRequest)
				return
			}

			owningOrg.Endpoint = append(owningOrg.Endpoint, epRef)

			var updatedOrg fhir.Organization
			err = client.Update("Organization/"+*owningOrg.Id, owningOrg, &updatedOrg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if strings.HasPrefix(forResourceStr, "HealthcareService/") {
			var owningService fhir.HealthcareService
			err = client.Read(forResourceStr, &owningService)
			if err != nil {
				http.Error(w, "bad request: could not find healthcare service", http.StatusBadRequest)
				return
			}

			owningService.Endpoint = append(owningService.Endpoint, epRef)

			var updatedService fhir.HealthcareService
			err = client.Update("HealthcareService/"+*owningService.Id, owningService, &updatedService)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusCreated)
	renderList[fhir.Endpoint, tmpls.EpListProps](client, w, tmpls.MakeEpListXsProps)
}

func newLocation(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)

	organizations, err := findAll[fhir.Organization](client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		PhysicalTypes []fhir.Coding
		Status        []fhir.Coding
		Types         []fhir.Coding
		Organizations []fhir.Organization
	}{
		PhysicalTypes: valuesets.LocationPhysicalTypeCodings,
		Status:        valuesets.LocationStatusCodings,
		Types:         valuesets.LocationTypeCodings,
		Organizations: organizations,
	}

	tmpls.RenderWithBase(w, "location_edit.html", props)
}

func newLocationPost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		badRequest(w, r, "invalid form input", err)
		return
	}

	location := fhir.Location{
		Meta: &fhir.Meta{
			Profile: []string{profile.NLGenericFunctionLocation},
		},
	}
	name := r.PostForm.Get("name")
	location.Name = &name

	typeCode := r.PostForm.Get("type")
	if len(typeCode) > 0 {
		locType, ok := valuesets.CodableFrom(valuesets.LocationTypeCodings, typeCode)
		if !ok {
			slog.WarnContext(r.Context(), "Could not find selected location type")
		} else {
			location.Type = []fhir.CodeableConcept{locType}
		}
	}

	statusCode := r.PostForm.Get("status")
	status, ok := valuesets.LocationStatusFrom(statusCode)
	if ok {
		location.Status = &status
	} else {
		slog.WarnContext(r.Context(), "Could not find location status")
	}

	var address fhir.Address
	addressLine := r.PostForm.Get("address-line")
	if addressLine == "" {
		http.Error(w, "missing address line", http.StatusBadRequest)
		return
	}
	address.Line = []string{addressLine}

	addressCity := r.PostForm.Get("address-city")
	if addressCity != "" {
		address.City = to.Ptr(addressCity)
	}
	addressDistrict := r.PostForm.Get("address-district")
	if addressDistrict != "" {
		address.District = to.Ptr(addressDistrict)
	}
	addressState := r.PostForm.Get("address-state")
	if addressState != "" {
		address.State = to.Ptr(addressState)
	}
	addressPostalCode := r.PostForm.Get("address-postal-code")
	if addressPostalCode != "" {
		address.PostalCode = to.Ptr(addressPostalCode)
	}
	addressCountry := r.PostForm.Get("address-country")
	if addressCountry != "" {
		address.Country = to.Ptr(addressCountry)
	}
	location.Address = to.Ptr(address)

	physicalCode := r.PostForm.Get("physicalType")
	if len(physicalCode) > 0 {
		physical, ok := valuesets.CodableFrom(valuesets.LocationPhysicalTypeCodings, physicalCode)
		if !ok {
			slog.WarnContext(r.Context(), "Could not find selected physical location type")
		} else {
			location.PhysicalType = &physical
		}
	}

	orgStr := r.PostForm.Get("managing-org")
	if orgStr != "" {
		reference := "Organization/" + orgStr
		refType := "Organization"
		location.ManagingOrganization = &fhir.Reference{
			Reference: &reference,
			Type:      &refType,
		}
		var managingOrg fhir.Organization
		err = client.Read(reference, &managingOrg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		location.ManagingOrganization.Display = managingOrg.Name
	}

	var resLoc fhir.Location
	err = client.Create(location, &resLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderList[fhir.Location, tmpls.LocationListProps](client, w, tmpls.MakeLocationListXsProps)
}

func listLocations(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderPaginatedList[fhir.Location, tmpls.LocationListProps](client, w, r, tmpls.MakeLocationListXsProps)
}

func newPractitionerRolePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		badRequest(w, r, "failed to processes form data", err)
		return
	}

	var role fhir.PractitionerRole
	uziNumber := r.PostForm.Get("uzi-number")
	if uziNumber != "" {
		identifier := fhir.Identifier{
			System: to.Ptr(coding.UZINamingSystem),
			Value:  to.Ptr(uziNumber),
		}
		ref := fhir.Reference{
			Identifier: to.Ptr(identifier),
		}
		role.Practitioner = to.Ptr(ref)
	} else {
		badRequest(w, r, "required field uzi-number missing", err)
		return
	}

	orgId := r.PostForm.Get("organization-id")
	org, err := findById[fhir.Organization](orgId)
	if err != nil {
		badRequest(w, r, fmt.Sprintf("could not find organistion with id: %s", orgId))
		return
	}
	orgRef := fhir.Reference{
		Reference: to.Ptr(fmt.Sprintf("Organization/%s", orgId)),
		Display:   org.Name,
	}
	role.Organization = to.Ptr(orgRef)

	codables, ok := formdata.CodablesFromForm(r.PostForm, valuesets.PractitionerRoleCodings, "codes")
	if !ok {
		badRequest(w, r, fmt.Sprintf("could not find all type codes"))
		return
	}
	role.Code = codables

	telecomData := formdata.ParseMaps(r.PostForm, "telecom")
	for _, tel := range telecomData {
		const msg = "invalid telecom information provided"
		system, ok := tel["System"]
		if !ok {
			badRequest(w, r, msg)
			return
		}
		value, ok := tel["Value"]
		if !ok {
			badRequest(w, r, msg)
			return
		}
		contactPointSystem, ok := valuesets.ContactPointSystemFrom(system)
		if !ok {
			badRequest(w, r, msg)
			return
		}

		contactPoint := fhir.ContactPoint{
			System: to.Ptr(contactPointSystem),
			Value:  to.Ptr(value),
		}

		role.Telecom = append(role.Telecom, contactPoint)
	}

	var resRole fhir.PractitionerRole
	err = client.Create(role, &resRole)
	if err != nil {
		internalError(w, r, "could not create practitioner role", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	renderList[fhir.PractitionerRole, tmpls.PractitionerRoleProps](client, w, tmpls.MakePractitionerRoleXsProps)
}

func newPractitionerRole(w http.ResponseWriter, r *http.Request) {
	organizations, err := findAll[fhir.Organization](client)
	if err != nil {
		internalError(w, r, "failed to load organizations", err)
		return
	}

	orgsExist := len(organizations) > 0

	props := struct {
		Organizations []fhir.Organization
		OrgsExist     bool
		Codes         []fhir.Coding
		TelecomCodes  []fhir.Coding
	}{
		Organizations: organizations,
		OrgsExist:     orgsExist,
		Codes:         valuesets.PractitionerRoleCodings,
		TelecomCodes:  valuesets.ContactPointSystem,
	}
	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "practitionerrole_edit.html", props)
}

func listPractitionerRole(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderPaginatedList[fhir.PractitionerRole, tmpls.PractitionerRoleProps](client, w, r, tmpls.MakePractitionerRoleXsProps)
}

func homePage(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "home.html", nil)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Path not implemented"))
}

func deleteHandler(resourceType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceId := r.PathValue("id")
		path := fmt.Sprintf("%s/%s", resourceType, resourceId)

		err := client.Delete(path)
		if err != nil {
			respondErrorAlert(w, fmt.Sprintf("Can not delete %s.", resourceType), http.StatusBadRequest)
			return
		}

		h := w.Header()
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("HX-Reswap", "delete")

		w.WriteHeader(http.StatusOK)
		return
	}
}

func findById[T any](id string) (T, error) {
	var prototype T
	resourceType := caramel.ResourceType(prototype)
	resourcePath := fmt.Sprintf("%s/%s", resourceType, id)

	err := client.Read(resourcePath, &prototype)
	return prototype, err
}

// PaginationInfo holds metadata about pagination state
type PaginationInfo struct {
	CurrentPage       int
	PageSize          int
	TotalItems        int
	TotalPages        int
	HasPrev           bool
	HasNext           bool
	PrevPage          int
	NextPage          int
	ShowingFrom       int
	ShowingTo         int
	PageWindow        []int
	ShowFirst         bool
	ShowFirstEllipsis bool
	ShowLast          bool
	ShowLastEllipsis  bool
}

// PaginatedResult contains paginated data and pagination metadata
type PaginatedResult[T any] struct {
	Items      []T
	Pagination PaginationInfo
}

const defaultPageSize = 20

func findAll[T any](fhirClient fhirclient.Client) ([]T, error) {
	var prototype T
	resourceType := caramel.ResourceType(prototype)

	var searchResponse fhir.Bundle
	err := fhirClient.Search(resourceType, url.Values{}, &searchResponse, nil)
	if err != nil {
		return nil, fmt.Errorf("search for resource type %s failed: %w", resourceType, err)
	}

	var result []T
	for i, entry := range searchResponse.Entry {
		var item T
		err := json.Unmarshal(entry.Resource, &item)
		if err != nil {
			return nil, fmt.Errorf("unmarshal of entry %d for resource type %s failed: %w", i, resourceType, err)
		}
		result = append(result, item)
	}

	return result, nil
}

// findPaginated retrieves paginated resources from FHIR server
// It uses _count, _total=accurate, and _getpagesoffset parameters for direct page access.
func findPaginated[T any](fhirClient fhirclient.Client, page, pageSize int) (PaginatedResult[T], error) {
	var prototype T
	resourceType := caramel.ResourceType(prototype)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}

	// Calculate offset: (page - 1) * pageSize
	// Page 1: offset = 0
	// Page 2: offset = pageSize
	// Page 3: offset = pageSize * 2, etc.
	offset := (page - 1) * pageSize

	searchParams := url.Values{}
	searchParams.Set("_count", strconv.Itoa(pageSize))
	searchParams.Set("_total", "accurate")
	if offset > 0 {
		searchParams.Set("_getpagesoffset", strconv.Itoa(offset))
	}

	var searchResponse fhir.Bundle
	err := fhirClient.Search(resourceType, searchParams, &searchResponse, nil)
	if err != nil {
		return PaginatedResult[T]{}, fmt.Errorf("search for resource type %s failed: %w", resourceType, err)
	}

	// Extract total count from the Bundle
	totalItems := 0
	if searchResponse.Total != nil {
		totalItems = *searchResponse.Total
	}

	// Unmarshal the entries for the current page
	pageItems := make([]T, 0, len(searchResponse.Entry))
	for i, entry := range searchResponse.Entry {
		var item T
		if err := json.Unmarshal(entry.Resource, &item); err != nil {
			return PaginatedResult[T]{}, fmt.Errorf("unmarshal of entry %d for resource type %s failed: %w", i, resourceType, err)
		}
		pageItems = append(pageItems, item)
	}

	// Calculate pagination metadata
	totalPages := totalItems / pageSize
	if totalItems%pageSize > 0 {
		totalPages++
	}
	if totalPages < 1 {
		totalPages = 1
	}

	showingFrom := 0
	if totalItems > 0 {
		showingFrom = (page-1)*pageSize + 1
	}
	showingTo := page * pageSize
	if showingTo > totalItems {
		showingTo = totalItems
	}

	pageWindow := make([]int, 0)
	for i := 1; i <= totalPages; i++ {
		if i >= page-2 && i <= page+2 {
			pageWindow = append(pageWindow, i)
		}
	}

	return PaginatedResult[T]{
		Items: pageItems,
		Pagination: PaginationInfo{
			CurrentPage:       page,
			PageSize:          pageSize,
			TotalItems:        totalItems,
			TotalPages:        totalPages,
			HasPrev:           page > 1,
			HasNext:           page < totalPages,
			PrevPage:          page - 1,
			NextPage:          page + 1,
			ShowingFrom:       showingFrom,
			ShowingTo:         showingTo,
			PageWindow:        pageWindow,
			ShowFirst:         page > 3,
			ShowFirstEllipsis: page > 4,
			ShowLast:          page < totalPages-2,
			ShowLastEllipsis:  page < totalPages-3,
		},
	}, nil
}

func uraIdentifier(uraString string) fhir.Identifier {
	var identifier fhir.Identifier
	identifier.Value = to.Ptr(uraString)
	identifier.System = to.Ptr(coding.URANamingSystem)
	return identifier
}

func renderList[R any, DTO any](fhirClient fhirclient.Client, httpResponse http.ResponseWriter, dtoFunc func([]R) []DTO) {
	resourceType := caramel.ResourceType(new(R))
	items, err := findAll[R](fhirClient)
	if err != nil {
		http.Error(httpResponse, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpls.RenderWithBase(httpResponse, strings.ToLower(resourceType)+"_list.html", struct {
		Items []DTO
	}{
		Items: dtoFunc(items),
	})
}

// renderPaginatedList renders a paginated list of resources
func renderPaginatedList[R any, DTO any](fhirClient fhirclient.Client, httpResponse http.ResponseWriter, httpRequest *http.Request, dtoFunc func([]R) []DTO) {
	resourceType := caramel.ResourceType(new(R))

	// Parse pagination parameters from query string
	page := 1
	pageSize := defaultPageSize

	if pageStr := httpRequest.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := httpRequest.URL.Query().Get("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	result, err := findPaginated[R](fhirClient, page, pageSize)
	if err != nil {
		http.Error(httpResponse, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpls.RenderWithBase(httpResponse, strings.ToLower(resourceType)+"_list.html", struct {
		Items      []DTO
		Pagination PaginationInfo
	}{
		Items:      dtoFunc(result.Items),
		Pagination: result.Pagination,
	})
}

func idFromRef(ref fhir.Reference) string {
	if ref.Reference == nil {
		return ""
	}

	split := strings.Split(*ref.Reference, "/")
	if len(split) != 2 {
		return ""
	}

	return split[1]
}

func ShortID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read never returns an error, and always fills b entirely.
		panic("unreachable")
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

func respondErrorAlert(w http.ResponseWriter, text string, httpcode int) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("HX-Retarget", "#alerts")
	h.Set("HX-Reswap", "beforeend")
	w.WriteHeader(httpcode)

	props := struct {
		AlertId string
		Text    string
	}{
		AlertId: ShortID(),
		Text:    text,
	}

	tmpls.RenderPartial(w, "_alert_error", props)
}

func respondErrorPage(w http.ResponseWriter, text string, httpcode int) {
	props := struct {
		AlertId string
		Text    string
	}{
		AlertId: ShortID(),
		Text:    text,
	}
	w.WriteHeader(httpcode)
	tmpls.RenderWithBase(w, "errorpage.html", props)
}

func internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	slog.ErrorContext(r.Context(), msg, logging.Error(err))

	isHtmxRequest := r.Header.Get("HX-Request") == "true"
	if isHtmxRequest {
		// Request is received from HTMX so we will assume rendering an error on the page
		respondErrorAlert(w, msg, http.StatusInternalServerError)
	} else {
		// No HTMX detected so let's just render the full error page
		respondErrorPage(w, msg, http.StatusInternalServerError)
	}
}

func badRequest(w http.ResponseWriter, r *http.Request, msg string, errs ...error) {
	hasError := len(errs) > 0
	if hasError {
		err := errs[0]
		slog.WarnContext(r.Context(), msg, logging.Error(err))
	}

	isHtmxRequest := r.Header.Get("HX-Request") == "true"
	if isHtmxRequest {
		// Request is received from HTMX so we will assume rendering an error on the page
		respondErrorAlert(w, msg, http.StatusBadRequest)
	} else {
		// No HTMX detected so let's just render the full error page
		respondErrorPage(w, msg, http.StatusBadRequest)
	}
}
