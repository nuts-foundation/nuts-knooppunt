package mitz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Config holds the configuration for the MITZ component
type Config struct {
	IsEnabled     bool   `koanf:"enabled"`
	FHIRBaseURL   string `koanf:"baseurl"`
	GatewaySystem string `koanf:"gateway_system"`
	SourceSystem  string `koanf:"source_system"`
}

func (c Config) Enabled() bool {
	return c.IsEnabled
}

// SubscribeRequest represents the parameters for creating a MITZ subscription
// todo: remove this once we have IG for Knooppunt integration
type SubscribeRequest struct {
	PatientID        string `json:"patient_id"`
	PatientBirthDate string `json:"patient_birth_date"`
	ProviderID       string `json:"provider_id"`
	ProviderType     string `json:"provider_type"`
	CallbackURL      string `json:"callback_url"`
}

var _ component.Lifecycle = (*Component)(nil)

// Component is the MITZ component that handles FHIR consent bundles
type Component struct {
	client        fhirclient.Client
	gatewaySystem string
	sourceSystem  string
}

// New creates a new MITZ component
func New(config Config) (*Component, error) {
	if config.FHIRBaseURL == "" {
		return nil, fmt.Errorf("FHIR base URL must be configured when MITZ component is enabled")
	}

	baseURL, err := url.Parse(config.FHIRBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid FHIR base URL: %w", err)
	}

	return &Component{
		client:        fhirclient.New(baseURL, http.DefaultClient, fhirutil.ClientConfig()),
		gatewaySystem: config.GatewaySystem,
		sourceSystem:  config.SourceSystem,
	}, nil
}

// RegisterHttpHandlers registers the HTTP handlers for the MITZ component
func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.Handle("POST /mitz/notify", http.HandlerFunc(c.handleNotify))
	internalMux.Handle("POST /mitz/subscribe", http.HandlerFunc(c.handleSubscribe))
}

// Start starts the component
func (c *Component) Start() error {
	return nil
}

// Stop stops the component
func (c *Component) Stop(ctx context.Context) error {
	return nil
}

// handleNotify handles FHIR consent bundle notifications
func (c *Component) handleNotify(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ParseRequest[fhir.Bundle](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	bundle := fhirRequest.Resource

	// Validate bundle type
	if bundle.Type != fhir.BundleTypeTransaction {
		err := &fhirapi.Error{
			Message:   "Bundle must be of type 'transaction'",
			IssueType: fhir.IssueTypeInvalid,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	// TODO: Process the bundle as needed
	// For now, just accept and return OK
	httpResponse.WriteHeader(http.StatusOK)
}

// handleSubscribe handles subscription creation requests
func (c *Component) handleSubscribe(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ParseRequest[SubscribeRequest](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	request := fhirRequest.Resource

	// Create FHIR Subscription
	subscription := c.createSubscription(request)

	// Send subscription to configured FHIR endpoint
	// The MITZ endpoint should respond with 202 Accepted
	var created fhir.Subscription
	err = c.client.CreateWithContext(httpRequest.Context(), subscription, &created)
	if err != nil {
		// Check if it's an OperationOutcome error to extract status code
		if outcomeErr, ok := err.(fhirclient.OperationOutcomeError); ok {
			switch outcomeErr.HttpStatusCode {
			case http.StatusBadRequest:
				err = &fhirapi.Error{
					Message:   "FHIR resource does not meet specifications",
					Cause:     err,
					IssueType: fhir.IssueTypeInvalid,
				}
			case http.StatusUnauthorized:
				err = &fhirapi.Error{
					Message:   "Not authorized to create subscription at MITZ endpoint",
					Cause:     err,
					IssueType: fhir.IssueTypeSecurity,
				}
			case http.StatusNotFound:
				err = &fhirapi.Error{
					Message:   "MITZ endpoint not found",
					Cause:     err,
					IssueType: fhir.IssueTypeNotFound,
				}
			case http.StatusUnprocessableEntity:
				err = &fhirapi.Error{
					Message:   "MITZ business rules are not met",
					Cause:     err,
					IssueType: fhir.IssueTypeBusinessRule,
				}
			case http.StatusTooManyRequests:
				err = &fhirapi.Error{
					Message:   "Too many requests to MITZ endpoint",
					Cause:     err,
					IssueType: fhir.IssueTypeThrottled,
				}
			default:
				err = &fhirapi.Error{
					Message:   "Failed to create subscription at MITZ endpoint",
					Cause:     err,
					IssueType: fhir.IssueTypeTransient,
				}
			}
		} else {
			err = &fhirapi.Error{
				Message:   "Failed to create subscription at MITZ endpoint",
				Cause:     err,
				IssueType: fhir.IssueTypeTransient,
			}
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	// MITZ should respond with 202 Accepted
	// Note: The fhir-client doesn't expose the raw HTTP response,
	// so we trust that if CreateWithContext succeeds, the subscription was accepted
	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusAccepted, created)
}

// createSubscription creates a FHIR Subscription from the subscribe request
func (c *Component) createSubscription(req SubscribeRequest) fhir.Subscription {
	subscription := fhir.Subscription{
		Status: fhir.SubscriptionStatusRequested,
		Reason: "OTV",
		Criteria: fmt.Sprintf("Consent?_query=otv&patientid=%s&providerid=%s&providertype=%s",
			req.PatientID, req.ProviderID, req.ProviderType),
		Channel: fhir.SubscriptionChannel{
			Type:     fhir.SubscriptionChannelTypeRestHook,
			Endpoint: to.Ptr(req.CallbackURL),
			Payload:  to.Ptr("application/fhir+xml"),
		},
	}

	// Add extensions
	var extensions []fhir.Extension

	// Patient birth date extension
	if req.PatientBirthDate != "" {
		extensions = append(extensions, fhir.Extension{
			Url:       "http://fhir.nl/StructureDefinition/Patient.birthDate",
			ValueDate: to.Ptr(req.PatientBirthDate),
		})
	}

	// Gateway system extension
	if c.gatewaySystem != "" {
		extensions = append(extensions, fhir.Extension{
			Url:      "http://fhir.nl/StructureDefinition/GatewaySystem",
			ValueOid: to.Ptr(c.gatewaySystem),
		})
	}

	// Source system extension
	if c.sourceSystem != "" {
		extensions = append(extensions, fhir.Extension{
			Url:      "http://fhir.nl/StructureDefinition/SourceSystem",
			ValueOid: to.Ptr(c.sourceSystem),
		})
	}

	if len(extensions) > 0 {
		subscription.Extension = extensions
	}

	return subscription
}
