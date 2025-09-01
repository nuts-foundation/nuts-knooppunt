package mcsdadmin

import (
	"context"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	tmpls "github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/templates"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Config struct {
	FHIRBaseURL string
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	config     Config
	fhirClient fhirclient.Client
}

// TODO: Make higher order handlers instead of having this globally
var client fhirclient.Client

func New(config Config) *Component {
	baseURL, err := url.Parse(config.FHIRBaseURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start MCSD admin component, invalid FHIRBaseURL")
	}

	client = fhirclient.New(baseURL, http.DefaultClient, fhirClientConfig())

	return &Component{
		config:     config,
		fhirClient: client,
	}
}

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	// Nothing to do
	return nil
}

// Route handling

func listServices(w http.ResponseWriter, r *http.Request) {
	services, err := FindAllServices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		Services []tmpls.ServiceListProps
	}{
		Services: tmpls.MakeServiceListXsProps(services),
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "service_list.html", props)
}

func newService(w http.ResponseWriter, r *http.Request) {
	organizations, err := FindAllOrganizations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	types, err := valuesets.CodingsFrom("service-type")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	props := struct {
		Types         []fhir.Coding
		Organizations []fhir.Organization
	}{
		Organizations: organizations,
		Types:         types,
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "service_edit.html", props)
}

func newServicePost(w http.ResponseWriter, r *http.Request) {
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

	typeCode := r.PostForm.Get("type")
	serviceType, ok := valuesets.CodableFrom("service-type", typeCode)
	if ok {
		service.Type = []fhir.CodeableConcept{serviceType}
	}

	reference := "Organization/" + r.PostForm.Get("providedById")
	refType := "Organization"
	service.ProvidedBy = &fhir.Reference{
		Reference: &reference,
		Type:      &refType,
	}

	var providedByOrg fhir.Organization
	err = client.Read(reference, &providedByOrg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find referred organisation")
		return
	}
	service.ProvidedBy.Display = providedByOrg.Name

	var resSer fhir.HealthcareService
	err = client.Create(service, &resSer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	services, err := FindAllServices()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all services")
	}

	props := struct {
		Services []tmpls.ServiceListProps
	}{
		Services: tmpls.MakeServiceListXsProps(services),
	}

	tmpls.RenderWithBase(w, "service_list.html", props)
}

func listOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := FindAllOrganizations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		Organizations []tmpls.OrgListProps
	}{
		Organizations: tmpls.MakeOrgListXsProps(orgs),
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "organization_list.html", props)
}

func newOrganization(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	types, err := valuesets.CodingsFrom("organization-type")
	if err != nil {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	props := struct {
		Types []fhir.Coding
	}{
		Types: types,
	}

	tmpls.RenderWithBase(w, "organization_edit.html", props)
}

func newOrganizationPost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for organization resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var org fhir.Organization
	name := r.PostForm.Get("name")
	org.Name = &name

	orgTypeCode := r.PostForm.Get("type")
	orgType, ok := valuesets.CodableFrom("organization-type", orgTypeCode)
	if ok {
		org.Type = []fhir.CodeableConcept{orgType}
	}

	active := r.PostForm.Get("active") == "true"
	org.Active = &active

	var resOrg fhir.Organization
	err = client.Create(org, &resOrg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	orgs, err := FindAllOrganizations()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all organizations")
	}

	props := struct {
		Organizations []tmpls.OrgListProps
	}{
		Organizations: tmpls.MakeOrgListXsProps(orgs),
	}

	tmpls.RenderWithBase(w, "organization_list.html", props)
}

func listEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := FindAllEndpoints()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		Endpoints []tmpls.EpListProps
	}{
		Endpoints: tmpls.MakeEpListXsProps(endpoints),
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "endpoint_list.html", props)
}

func newEndpoint(w http.ResponseWriter, r *http.Request) {
	organizations, err := FindAllOrganizations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status, err := valuesets.CodingsFrom("endpoint-status")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payloadTypes, err := valuesets.CodingsFrom("endpoint-payload-type")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	connectionTypes, err := valuesets.CodingsFrom("endpoint-connection-type")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	purposeOfUse, err := valuesets.CodingsFrom("purpose-of-use")
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
		ConnectionTypes: connectionTypes,
		Organizations:   organizations,
		PayloadTypes:    payloadTypes,
		PurposeOfUse:    purposeOfUse,
		Status:          status,
	}

	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "endpoint_edit.html", props)
}

func newEndpointPost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for Endpoint resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var endpoint fhir.Endpoint
	address := r.PostForm.Get("address")
	endpoint.Address = address

	var payloadType fhir.CodeableConcept
	payloadTypeId := r.PostForm.Get("payload-type")
	payloadType, ok := valuesets.CodableFrom("endpoint-payload-type", payloadTypeId)
	if ok {
		endpoint.PayloadType = []fhir.CodeableConcept{payloadType}
	} else {
		log.Warn().Msg("Failed to find referred payload type")
	}

	periodStart := r.PostForm.Get("period-start")
	periodEnd := r.PostForm.Get("period-end")
	if (len(periodStart) > 0) && (len(periodEnd) > 0) {
		endpoint.Period = &fhir.Period{
			Start: &periodStart,
			End:   &periodEnd,
		}
	} else {
		log.Warn().Msg("Missing period")
	}

	contactValue := r.PostForm.Get("contact")
	if len(contactValue) > 0 {
		contact := fhir.ContactPoint{
			Value: &contactValue,
		}
		endpoint.Contact = []fhir.ContactPoint{contact}
	} else {
		log.Warn().Msg("Missing contact value")
	}

	orgFormStr := r.PostForm.Get("managing-org")
	if len(orgFormStr) > 0 {
		var managingOrg fhir.Organization
		reference := "Organization/" + orgFormStr
		refType := "Organization"
		endpoint.ManagingOrganization = &fhir.Reference{
			Reference: &reference,
			Type:      &refType,
		}
		err = client.Read(reference, &managingOrg)
		if err != nil {
			log.Error().Err(err).Msg("Failed to find referred organisation")
			return
		}
		endpoint.ManagingOrganization.Display = managingOrg.Name
	} else {
		log.Warn().Msg("Missing organisation value")
	}

	var connectionType fhir.Coding
	connectionTypeId := r.PostForm.Get("connection-type")
	connectionType, ok = valuesets.CodingFrom("endpoint-connection-type", connectionTypeId)
	if ok {
		endpoint.ConnectionType = connectionType
	} else {
		log.Warn().Msg("Failed to find referred connection type")
	}

	purposeOfUseId := r.PostForm.Get("purpose-of-use")
	purposeOfUse, ok := valuesets.CodableFrom("purpose-of-use", purposeOfUseId)
	if ok {
		extension := fhir.Extension{
			Url:                  "https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.PurposeOfUse",
			ValueCodeableConcept: &purposeOfUse,
		}
		endpoint.Extension = append(endpoint.Extension, extension)
	} else {
		log.Warn().Msg("Failed to find referred purpose of use")
	}

	status := r.PostForm.Get("status")
	endpoint.Status, ok = valuesets.EndpointStatusFrom(status)
	if !ok {
		log.Warn().Msg("Failed to determine status, default to active")
	}

	var resEp fhir.Endpoint
	err = client.Create(endpoint, &resEp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	endpoints, err := FindAllEndpoints()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all endpoints")
	}

	props := struct {
		Endpoints []tmpls.EpListProps
	}{
		Endpoints: tmpls.MakeEpListXsProps(endpoints),
	}

	tmpls.RenderWithBase(w, "endpoint_list.html", props)
}

func newLocation(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	locationTypes, err := valuesets.CodingsFrom("location-type")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find location types")
	}

	physicalTypes, err := valuesets.CodingsFrom("location-physical-type")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find physical location types")
	}

	status, err := valuesets.CodingsFrom("location-status")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find location status types")
	}

	props := struct {
		PhysicalTypes []fhir.Coding
		Status        []fhir.Coding
		Types         []fhir.Coding
	}{
		PhysicalTypes: physicalTypes,
		Status:        status,
		Types:         locationTypes,
	}

	tmpls.RenderWithBase(w, "location_edit.html", props)
}

func newLocationPost(w http.ResponseWriter, r *http.Request) {
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
		locType, ok := valuesets.CodableFrom("location-type", typeCode)
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
		physical, ok := valuesets.CodableFrom("location-physical-type", physicalCode)
		if !ok {
			log.Warn().Msg("Could not find selected physical location type")
		} else {
			location.PhysicalType = &physical
		}
	}

	var resLoc fhir.Location
	err = client.Create(location, &resLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpls.RenderWithBase(w, "location_list.html", nil)
}

func listLocations(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	locs, err := FindAllLocations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		Locations []tmpls.LocationListProps
	}{
		Locations: tmpls.MakeLocationListXsProps(locs),
	}

	tmpls.RenderWithBase(w, "location_list.html", props)
}

func homePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	tmpls.RenderWithBase(w, "home.html", nil)
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	tmpls.RenderWithBase(w, "home.html", nil)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Path not implemented"))
}

func (c Component) RegisterHttpHandlers(mux *http.ServeMux, _ *http.ServeMux) {
	mux.HandleFunc("GET /mcsdadmin/healthcareservice", listServices)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/new", newService)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/new", newServicePost)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/{id}/edit", notImplemented)
	mux.HandleFunc("PUT /mcsdadmin/healthcareservice/{id}/edit", notImplemented)
	mux.HandleFunc("GET /mcsdadmin/organization", listOrganizations)
	mux.HandleFunc("GET /mcsdadmin/organization/new", newOrganization)
	mux.HandleFunc("POST /mcsdadmin/organization/new", newOrganizationPost)
	mux.HandleFunc("GET /mcsdadmin/endpoint", listEndpoints)
	mux.HandleFunc("GET /mcsdadmin/endpoint/new", newEndpoint)
	mux.HandleFunc("POST /mcsdadmin/endpoint/new", newEndpointPost)
	mux.HandleFunc("GET /mcsdadmin/location", listLocations)
	mux.HandleFunc("GET /mcsdadmin/location/new", newLocation)
	mux.HandleFunc("POST /mcsdadmin/location/new", newLocationPost)
	mux.HandleFunc("GET /mcsdadmin", homePage)
	mux.HandleFunc("GET /mcsdadmin/", notFound)
}
