package mitz

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/xacml"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Config holds the configuration for the MITZ component
type Config struct {
	MitzBase       string `koanf:"mitzbase"`
	GatewaySystem  string `koanf:"gateway_system"`
	SourceSystem   string `koanf:"source_system"`
	NotifyEndpoint string `koanf:"notify_endpoint"`  // Endpoint URL for subscription notifications
	TLSCertFile    string `koanf:"tls_cert_file"`    // PEM certificate file OR .p12/.pfx file
	TLSKeyFile     string `koanf:"tls_key_file"`     // PEM key file (not used if TLSCertFile is .p12/.pfx)
	TLSKeyPassword string `koanf:"tls_key_password"` // Password for encrypted key or .p12/.pfx file
	TLSCAFile      string `koanf:"tls_ca_file"`      // CA certificate file to verify MITZ server
}

func (c Config) Enabled() bool {
	return c.MitzBase != ""
}

var _ component.Lifecycle = (*Component)(nil)

// Component is the MITZ component that handles FHIR consent bundles
type Component struct {
	client               fhirclient.Client
	httpClient           *http.Client
	consentCheckEndpoint string
	gatewaySystem        string
	sourceSystem         string
	notifyEndpoint       string
}

const (
	subscriptionPath = "/abonnementen/fhir"
	consentCheckPath = "/geslotenautorisatievraag/xacml3"
)

// New creates a new MITZ component
func New(config Config) (*Component, error) {
	if config.MitzBase == "" {
		return nil, fmt.Errorf("mitzbase must be configured when MITZ component is enabled")
	}

	// Parse base URL and construct subscription endpoint
	baseURL, err := url.Parse(config.MitzBase)
	if err != nil {
		return nil, fmt.Errorf("invalid mitzbase URL: %w", err)
	}
	subscriptionURL := baseURL.JoinPath(subscriptionPath)

	// Create HTTP client with optional mTLS configuration
	httpClient, err := createHTTPClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	consentCheckEndpoint := baseURL.JoinPath(consentCheckPath).String()

	return &Component{
		client:               fhirclient.New(subscriptionURL, httpClient, fhirutil.ClientConfig()),
		httpClient:           httpClient,
		consentCheckEndpoint: consentCheckEndpoint,
		gatewaySystem:        config.GatewaySystem,
		sourceSystem:         config.SourceSystem,
		notifyEndpoint:       config.NotifyEndpoint,
	}, nil
}

// createHTTPClient creates an HTTP client with optional mTLS configuration
func createHTTPClient(config Config) (*http.Client, error) {
	client := &http.Client{
		Transport: http.DefaultTransport,
	}

	// If TLS certificate is configured, set up mTLS
	if config.TLSCertFile != "" {
		tlsConfig, err := tlsutil.CreateTLSConfig(
			config.TLSCertFile,
			config.TLSKeyFile,
			config.TLSKeyPassword,
			config.TLSCAFile,
		)
		if err != nil {
			// Fail early when TLS is explicitly configured but setup fails
			return nil, fmt.Errorf("TLS is configured but failed to load: %w", err)
		}

		// Create transport with TLS config
		client.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		log.Info().Str("certFile", config.TLSCertFile).Msg("Successfully configured mTLS for MITZ connection")
	} else {
		log.Info().Msg("No TLS certificate configured, using plain HTTP client for MITZ connection")
	}

	return client, nil
}

// RegisterHttpHandlers registers the HTTP handlers for the MITZ component
func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.Handle("POST /mitz/notify", http.HandlerFunc(c.handleNotify))
	internalMux.Handle("POST /mitz/Subscription", http.HandlerFunc(c.handleSubscribe))
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
	log.Info().Msg("Received FHIR consent bundle notification")

	// todo: process it? atm we don't care about it. If we will care, we may have a problem because they seem
	// to be sending XMLs, which go fhir lib doesn't support yet

	log.Info().Msg("Successfully processed consent bundle notification")
	httpResponse.WriteHeader(http.StatusOK)
}

// CreateSubscription creates a MITZ subscription (implements nvi.MITZSubscriber interface)
func (c *Component) CreateSubscription(ctx context.Context, patientID, providerID, providerType string) error {
	// Create FHIR Subscription
	subscription := c.createSubscription(patientID, providerID, providerType)

	// Send subscription to configured FHIR endpoint
	var created fhir.Subscription
	var headers fhirclient.Headers
	err := c.client.CreateWithContext(ctx, subscription, &created, fhirclient.ResponseHeaders(&headers))
	if err != nil {
		// Check if it's an OperationOutcome error to extract status code
		if outcomeErr, ok := err.(fhirclient.OperationOutcomeError); ok {
			switch outcomeErr.HttpStatusCode {
			case http.StatusBadRequest:
				return fmt.Errorf("FHIR resource does not meet specifications: %w", err)
			case http.StatusUnauthorized:
				return fmt.Errorf("not authorized to create subscription at MITZ endpoint: %w", err)
			case http.StatusNotFound:
				return fmt.Errorf("MITZ endpoint not found: %w", err)
			case http.StatusUnprocessableEntity:
				return fmt.Errorf("MITZ business rules are not met: %w", err)
			case http.StatusTooManyRequests:
				return fmt.Errorf("too many requests to MITZ endpoint: %w", err)
			default:
				return fmt.Errorf("failed to create subscription at MITZ endpoint: %w", err)
			}
		}
	}
	// 202 Accepted is OK (MITZ responds with 202 instead of 201)

	location := headers.Header.Get("Location")

	log.Info().
		Str("patientID", patientID).
		Str("providerID", providerID).
		Str("subscriptionId", location).
		Msg("Successfully created MITZ subscription")

	return nil
}

// handleConsentCheck triggers a consent check by invoking MITZ closed query
func (c *Component) handleConsentCheck(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	if c.consentCheckEndpoint == "" {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   "Consent check endpoint not configured",
			IssueType: fhir.IssueTypeNotSupported,
		})
		return
	}

	// Create a request with all required parameters
	req := xacml.AuthzRequest{
		PatientBSN:             "900186021",
		HealthcareFacilityType: "Z3",
		AuthorInstitutionID:    "00000659",
		EventCode:              "GGC002",
		SubjectRole:            "01.015",
		ProviderID:             "000095254",
		ProviderInstitutionID:  "00000666",
		ConsultingFacilityType: "Z3",
		PurposeOfUse:           "TREAT",
		ToAddress:              "http://localhost:8000/4",
	}

	authnDecisionQueryXml, err := xacml.CreateAuthzDecisionQuery(req)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   "Failed to create authorization decision query",
			Cause:     err,
			IssueType: fhir.IssueTypeInvalid,
		})
		return
	}

	// Log the XML request
	log.Info().
		Str("endpoint", c.consentCheckEndpoint).
		Str("xmlPayload", authnDecisionQueryXml).
		Msg("Sending consent check request to MITZ")

	// Create HTTP request with XML payload
	httpReq, err := http.NewRequestWithContext(
		httpRequest.Context(),
		http.MethodPost,
		c.consentCheckEndpoint,
		bytes.NewBufferString(authnDecisionQueryXml),
	)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   "Failed to create HTTP request",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		})
		return
	}

	// Set XML content type
	httpReq.Header.Set("Content-Type", "text/xml")

	// Send request using mTLS-configured HTTP client
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send consent check request to MITZ")
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   "Failed to send consent check request",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		})
		return
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read consent check response")
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   "Failed to read consent check response",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		})
		return
	}

	// Log response
	log.Info().
		Int("statusCode", resp.StatusCode).
		Str("responseBody", string(responseBody)).
		Msg("Received consent check response from MITZ")

	// Check for non-2xx status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   fmt.Sprintf("Consent check failed with status %d: %s", resp.StatusCode, string(responseBody)),
			IssueType: fhir.IssueTypeTransient,
		})
		return
	}

	// Return the XML response
	httpResponse.Header().Set("Content-Type", "text/xml")
	httpResponse.WriteHeader(http.StatusOK)
	httpResponse.Write(responseBody)
}

// validateMITZSubscription validates that a Subscription resource meets MITZ requirements
func validateMITZSubscription(subscription fhir.Subscription) *fhirapi.Error {
	// Validate status
	if subscription.Status != fhir.SubscriptionStatusRequested {
		return &fhirapi.Error{
			Message:   fmt.Sprintf("Subscription.status must be 'requested', got '%s'", subscription.Status),
			IssueType: fhir.IssueTypeValue,
		}
	}

	// Validate reason
	if subscription.Reason != "OTV" {
		return &fhirapi.Error{
			Message:   fmt.Sprintf("Subscription.reason must be 'OTV', got '%s'", subscription.Reason),
			IssueType: fhir.IssueTypeValue,
		}
	}

	// Validate criteria format
	if !strings.HasPrefix(subscription.Criteria, "Consent?_query=otv&") {
		return &fhirapi.Error{
			Message:   "Subscription.criteria must start with 'Consent?_query=otv&'",
			IssueType: fhir.IssueTypeValue,
		}
	}

	// Validate criteria contains required parameters
	if !strings.Contains(subscription.Criteria, "patientid=") {
		return &fhirapi.Error{
			Message:   "Subscription.criteria must contain 'patientid' parameter",
			IssueType: fhir.IssueTypeValue,
		}
	}
	if !strings.Contains(subscription.Criteria, "providerid=") {
		return &fhirapi.Error{
			Message:   "Subscription.criteria must contain 'providerid' parameter",
			IssueType: fhir.IssueTypeValue,
		}
	}
	if !strings.Contains(subscription.Criteria, "providertype=") {
		return &fhirapi.Error{
			Message:   "Subscription.criteria must contain 'providertype' parameter",
			IssueType: fhir.IssueTypeValue,
		}
	}

	// Validate channel
	if subscription.Channel.Type != fhir.SubscriptionChannelTypeRestHook {
		return &fhirapi.Error{
			Message:   fmt.Sprintf("Subscription.channel.type must be 'rest-hook', got '%s'", subscription.Channel.Type),
			IssueType: fhir.IssueTypeValue,
		}
	}

	for _, ext := range subscription.Extension {
		// Only allow these extensions
		if ext.Url != "http://fhir.nl/StructureDefinition/Patient.birthDate" &&
			ext.Url != "http://fhir.nl/StructureDefinition/GatewaySystem" &&
			ext.Url != "http://fhir.nl/StructureDefinition/SourceSystem" {
			return &fhirapi.Error{
				Message:   fmt.Sprintf("Unsupported extension URL: %s", ext.Url),
				IssueType: fhir.IssueTypeNotSupported,
			}
		}
	}

	return nil
}

// addConfigExtensions adds GatewaySystem and SourceSystem extensions from config to the subscription
func (c *Component) addConfigExtensions(subscription *fhir.Subscription) {
	// Gateway system extension
	if c.gatewaySystem != "" {
		subscription.Extension = append(subscription.Extension, fhir.Extension{
			Url:      "http://fhir.nl/StructureDefinition/GatewaySystem",
			ValueOid: to.Ptr(c.gatewaySystem),
		})
	}

	// Source system extension
	if c.sourceSystem != "" {
		subscription.Extension = append(subscription.Extension, fhir.Extension{
			Url:      "http://fhir.nl/StructureDefinition/SourceSystem",
			ValueOid: to.Ptr(c.sourceSystem),
		})
	}
}

// handleSubscribe handles subscription creation requests where payload is already Mitz compliant Consent
func (c *Component) handleSubscribe(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ParseRequest[fhir.Subscription](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	resource := fhirRequest.Resource

	// Validate the subscription resource
	if err := validateMITZSubscription(resource); err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	// Add gateway and source system extensions from config
	c.addConfigExtensions(&resource)

	// Set default payload if not provided
	if resource.Channel.Payload == nil || *resource.Channel.Payload == "" {
		resource.Channel.Payload = to.Ptr("application/fhir+json")
		log.Ctx(httpRequest.Context()).Debug().Msg("Set default channel payload to application/fhir+json")
	}

	// Use endpoint from configuration if not already provided in the request
	if resource.Channel.Endpoint == nil || *resource.Channel.Endpoint == "" {
		if c.notifyEndpoint != "" {
			resource.Channel.Endpoint = to.Ptr(c.notifyEndpoint)
			log.Ctx(httpRequest.Context()).Debug().Str("endpoint", c.notifyEndpoint).Msg("Set subscription channel endpoint from configuration")
		} else {
			log.Ctx(httpRequest.Context()).Warn().Msg("No subscription notify endpoint configured")
		}
	} else {
		log.Ctx(httpRequest.Context()).Debug().Str("endpoint", *resource.Channel.Endpoint).Msg("Using channel endpoint from incoming subscription")
	}

	// Send subscription to configured FHIR endpoint
	// The MITZ endpoint should respond with 202 Accepted
	// Note: The go-fhir-client library only supports JSON, not XML
	// XML support would require manually constructing the HTTP request
	var headers fhirclient.Headers
	err = c.client.CreateWithContext(httpRequest.Context(), resource, nil, fhirclient.ResponseHeaders(&headers))
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
			fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
			return
		}
	}

	location := headers.Header.Get("Location")
	// Extract ID from Location header (e.g., "Subscription/8904A5ED-713A-4A63-9B24-954AC7B7052D" -> "8904A5ED-713A-4A63-9B24-954AC7B7052D")
	if location != "" {
		parts := strings.Split(location, "/")
		if len(parts) > 1 {
			resource.Id = to.Ptr(parts[len(parts)-1])
		}
	}

	// MITZ should respond with 202 Accepted
	// Note: The fhir-client doesn't expose the raw HTTP response,
	// so we trust that if CreateWithContext succeeds, the subscription was accepted
	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusCreated, resource)
}

// createSubscription creates a FHIR Subscription from the subscribe request
func (c *Component) createSubscription(patientId, providerId, providerType string) fhir.Subscription {
	subscription := fhir.Subscription{
		Status: fhir.SubscriptionStatusRequested,
		Reason: "OTV",
		Criteria: fmt.Sprintf("Consent?_query=otv&patientid=%s&providerid=%s&providertype=%s",
			patientId, providerId, providerType),
		Channel: fhir.SubscriptionChannel{
			Type:    fhir.SubscriptionChannelTypeRestHook,
			Payload: to.Ptr("application/fhir+json"),
		},
	}

	// Add extensions
	var extensions []fhir.Extension

	/**
	This data is
	conditionally mandatory: if the
	healthcare provider has a verified
	date of birth of the patient, then it
	must be given.

	Patient birth date extension
	if req.PatientBirthDate != "" {
		extensions = append(extensions, fhir.Extension{
			Url:       "http://fhir.nl/StructureDefinition/Patient.birthDate",
			ValueDate: to.Ptr(req.PatientBirthDate),
		})
	}

	*/

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
