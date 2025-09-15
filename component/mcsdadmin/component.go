package mcsdadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/static"
	tmpls "github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/templates"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/rs/zerolog/log"
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

func New(config Config) *Component {
	baseURL, err := url.Parse(config.FHIRBaseURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start MCSD admin component, invalid FHIRBaseURL")
		return nil
	}

	return &Component{
		config:     config,
		fhirClient: fhirclient.New(baseURL, http.DefaultClient, fhirClientConfig()),
	}
}

func (c *Component) Start() error {
	// Nothing to do
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	// Nothing to do
	return nil
}

func (c *Component) RegisterHttpHandlers(mux *http.ServeMux, _ *http.ServeMux) {
	// Static file serving for CSS and fonts
	var assetHandler = http.StripPrefix("/mcsdadmin/", http.FileServer(http.FS(static.FS)))
	mux.Handle("GET /mcsdadmin/css/", assetHandler)
	mux.Handle("GET /mcsdadmin/js/", assetHandler)
	mux.Handle("GET /mcsdadmin/webfonts/", assetHandler)

	mux.HandleFunc("GET /mcsdadmin/healthcareservice", c.listServices)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/new", c.newService)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/new", c.newServicePost)
	mux.HandleFunc("GET /mcsdadmin/organization", c.listOrganizations)
	mux.HandleFunc("GET /mcsdadmin/organization/new", c.newOrganization)
	mux.HandleFunc("POST /mcsdadmin/organization/new", c.newOrganizationPost)
	mux.HandleFunc("GET /mcsdadmin/endpoint", c.listEndpoints)
	mux.HandleFunc("GET /mcsdadmin/endpoint/new", c.newEndpoint)
	mux.HandleFunc("POST /mcsdadmin/endpoint/new", c.newEndpointPost)
	mux.HandleFunc("GET /mcsdadmin/location", c.listLocations)
	mux.HandleFunc("GET /mcsdadmin/location/new", c.newLocation)
	mux.HandleFunc("POST /mcsdadmin/location/new", c.newLocationPost)
	mux.HandleFunc("DELETE /mcsdadmin/endpoint/{id}", c.deleteHandler("Endpoint"))
	mux.HandleFunc("DELETE /mcsdadmin/location/{id}", c.deleteHandler("Location"))
	mux.HandleFunc("DELETE /mcsdadmin/healthcareservice/{id}", c.deleteHandler("HealthcareService"))
	mux.HandleFunc("DELETE /mcsdadmin/organization/{id}", c.deleteHandler("Organization"))
	mux.HandleFunc("GET /mcsdadmin", c.homePage)
	mux.HandleFunc("GET /mcsdadmin/", c.notFound)
}

func (c *Component) listServices(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderList[fhir.HealthcareService, tmpls.ServiceListProps](c.fhirClient, w, tmpls.MakeServiceListXsProps)
}

func (c *Component) newService(w http.ResponseWriter, _ *http.Request) {
	organizations, err := findAll[fhir.Organization](c.fhirClient)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func (c *Component) newServicePost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for HealthcareService resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var service fhir.HealthcareService
	name := r.PostForm.Get("name")
	service.Name = &name
	active := r.PostForm.Get("active") == "true"
	service.Active = &active

	typeCodes := r.PostForm["type"]
	typeCodesCount := len(typeCodes)
	if typeCodesCount > 0 {
		service.Type = make([]fhir.CodeableConcept, typeCodesCount)
		for i, t := range typeCodes {
			serviceType, ok := valuesets.CodableFrom(valuesets.ServiceTypeCodings, t)
			if ok {
				service.Type[i] = serviceType
			} else {
				http.Error(w, fmt.Sprintf("Could not find type code %s", t), http.StatusBadRequest)
				return
			}
		}
	}

	reference := "Organization/" + r.PostForm.Get("providedById")
	refType := "Organization"
	service.ProvidedBy = &fhir.Reference{
		Reference: &reference,
		Type:      &refType,
	}

	var providedByOrg fhir.Organization
	err = c.fhirClient.Read(reference, &providedByOrg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find referred organisation")
		return
	}
	service.ProvidedBy.Display = providedByOrg.Name

	var resSer fhir.HealthcareService
	err = c.fhirClient.Create(service, &resSer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	renderList[fhir.HealthcareService, tmpls.ServiceListProps](c.fhirClient, w, tmpls.MakeServiceListXsProps)
}

func (c *Component) listOrganizations(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderList[fhir.Organization, tmpls.OrgListProps](c.fhirClient, w, tmpls.MakeOrgListXsProps)
}

func (c *Component) newOrganization(w http.ResponseWriter, _ *http.Request) {
	organizations, err := findAll[fhir.Organization](c.fhirClient)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func (c *Component) newOrganizationPost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for organization resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var org fhir.Organization
	name := r.PostForm.Get("name")
	org.Name = &name
	uraString := r.PostForm.Get("identifier")
	if uraString == "" {
		http.Error(w, "Bad request: missing URA identifier", http.StatusBadRequest)
		return
	}
	org.Identifier = []fhir.Identifier{
		uraIdentifier(uraString),
	}

	orgTypeCodes := r.PostForm["type"]
	typeCodesCount := len(orgTypeCodes)
	if typeCodesCount > 0 {
		org.Type = make([]fhir.CodeableConcept, 0, typeCodesCount)
		for _, t := range orgTypeCodes {
			if t == "" {
				continue
			}
			orgType, ok := valuesets.CodableFrom(valuesets.OrganizationTypeCodings, t)
			if ok {
				org.Type = append(org.Type, orgType)
			} else {
				http.Error(w, fmt.Sprintf("could not find type code %s", t), http.StatusBadRequest)
				return
			}
		}
	}

	active := r.PostForm.Get("active") == "true"
	org.Active = &active

	partOf := r.PostForm.Get("part-of")
	if len(partOf) > 0 {
		reference := "Organization/" + partOf
		org.PartOf = &fhir.Reference{
			Reference: &reference,
			Type:      to.Ptr("Organization"),
		}
		var parentOrg fhir.Organization
		err = c.fhirClient.Read(reference, &parentOrg)
		if err != nil {
			http.Error(w, "internal error: could not find organization", http.StatusInternalServerError)
			return
		}
		org.PartOf.Display = parentOrg.Name
	}

	var resOrg fhir.Organization
	err = c.fhirClient.Create(org, &resOrg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	renderList[fhir.Organization, tmpls.OrgListProps](c.fhirClient, w, tmpls.MakeOrgListXsProps)
}

func (c *Component) listEndpoints(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderList[fhir.Endpoint, tmpls.EpListProps](c.fhirClient, w, tmpls.MakeEpListXsProps)
}

func (c *Component) newEndpoint(w http.ResponseWriter, _ *http.Request) {
	organizations, err := findAll[fhir.Organization](c.fhirClient)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		ConnectionTypes []fhir.Coding
		Organizations   []fhir.Organization
		PayloadTypes    []fhir.Coding
		PurposeOfUse    []fhir.Coding
		Status          []fhir.Coding
	}{
		ConnectionTypes: valuesets.EndpointConnectionTypeCodings,
		Organizations:   organizations,
		PayloadTypes:    valuesets.EndpointPayloadTypeCodings,
		PurposeOfUse:    valuesets.PurposeOfUseCodings,
		Status:          valuesets.EndpointStatusCodings,
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "endpoint_edit.html", props)
}

func (c *Component) newEndpointPost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for Endpoint resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var endpoint fhir.Endpoint
	address := r.PostForm.Get("address")
	if address == "" {
		http.Error(w, "bad request: missing address", http.StatusBadRequest)
		return
	}
	endpoint.Address = address

	typeCodes := r.PostForm["payload-type"]
	typeCodesCount := len(typeCodes)
	if typeCodesCount > 0 {
		endpoint.PayloadType = make([]fhir.CodeableConcept, typeCodesCount)
		for i, t := range typeCodes {
			serviceType, ok := valuesets.CodableFrom(valuesets.EndpointPayloadTypeCodings, t)
			if ok {
				endpoint.PayloadType[i] = serviceType
			} else {
				http.Error(w, fmt.Sprintf("Could not find type code %s", t), http.StatusBadRequest)
			}
		}
	} else {
		http.Error(w, "missing payload type", http.StatusBadRequest)
	}

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
	connectionType, ok := valuesets.CodingFrom(valuesets.EndpointConnectionTypeCodings, connectionTypeId)
	if ok {
		endpoint.ConnectionType = connectionType
	} else {
		http.Error(w, "bad request: missing/invalid connection type", http.StatusBadRequest)
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

	forOrgStr := r.PostForm.Get("endpoint-for")
	var owningOrg fhir.Organization
	if len(forOrgStr) > 0 {
		err = c.fhirClient.Read("Organization/"+forOrgStr, &owningOrg)
		if err != nil {
			http.Error(w, "bad request: could not find organization", http.StatusBadRequest)
			return
		}
	}

	var resEp fhir.Endpoint
	err = c.fhirClient.Create(endpoint, &resEp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var epRef fhir.Reference
	epRef.Type = to.Ptr("Endpoint")
	epRef.Reference = to.Ptr("Endpoint/" + *resEp.Id)

	owningOrg.Endpoint = append(owningOrg.Endpoint, epRef)

	var updatedOrg fhir.Organization
	err = c.fhirClient.Update("Organization/"+*owningOrg.Id, owningOrg, &updatedOrg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	renderList[fhir.Endpoint, tmpls.EpListProps](c.fhirClient, w, tmpls.MakeEpListXsProps)
}

func (c *Component) newLocation(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)

	props := struct {
		PhysicalTypes []fhir.Coding
		Status        []fhir.Coding
		Types         []fhir.Coding
	}{
		PhysicalTypes: valuesets.LocationPhysicalTypeCodings,
		Status:        valuesets.LocationStatusCodings,
		Types:         valuesets.LocationTypeCodings,
	}

	tmpls.RenderWithBase(w, "location_edit.html", props)
}

func (c *Component) newLocationPost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var location fhir.Location
	name := r.PostForm.Get("name")
	location.Name = &name

	typeCode := r.PostForm.Get("type")
	if len(typeCode) > 0 {
		locType, ok := valuesets.CodableFrom(valuesets.LocationTypeCodings, typeCode)
		if !ok {
			log.Warn().Msg("Could not find selected location type")
		} else {
			location.Type = []fhir.CodeableConcept{locType}
		}
	}

	statusCode := r.PostForm.Get("status")
	status, ok := valuesets.LocationStatusFrom(statusCode)
	if ok {
		location.Status = &status
	} else {
		log.Warn().Msg("Could not find location status")
	}

	physicalCode := r.PostForm.Get("physicalType")
	if len(physicalCode) > 0 {
		physical, ok := valuesets.CodableFrom(valuesets.LocationPhysicalTypeCodings, physicalCode)
		if !ok {
			log.Warn().Msg("Could not find selected physical location type")
		} else {
			location.PhysicalType = &physical
		}
	}

	var resLoc fhir.Location
	err = c.fhirClient.Create(location, &resLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderList[fhir.Location, tmpls.LocationListProps](c.fhirClient, w, tmpls.MakeLocationListXsProps)
}

func (c *Component) listLocations(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderList[fhir.Location, tmpls.LocationListProps](c.fhirClient, w, tmpls.MakeLocationListXsProps)
}

func (c *Component) homePage(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "home.html", nil)
}

func (c *Component) notFound(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Path not implemented"))
}

func (c *Component) deleteHandler(resourceType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceId := r.PathValue("id")
		path := fmt.Sprintf("%s/%s", resourceType, resourceId)

		err := c.fhirClient.Delete(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}
}

func fhirClientConfig() *fhirclient.Config {
	config := fhirclient.DefaultConfig()
	config.DefaultOptions = []fhirclient.Option{
		fhirclient.RequestHeaders(map[string][]string{
			"Cache-Control": {"no-cache"},
		}),
	}
	config.Non2xxStatusHandler = func(response *http.Response, responseBody []byte) {
		log.Debug().Msgf("Non-2xx status code from FHIR server (%s %s, status=%d), content: %s", response.Request.Method, response.Request.URL, response.StatusCode, string(responseBody))
	}
	return &config
}

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

func uraIdentifier(uraString string) fhir.Identifier {
	var identifier fhir.Identifier
	identifier.Value = to.Ptr(uraString)
	identifier.System = to.Ptr(coding.URANamingSystem)
	return identifier
}
