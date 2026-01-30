package pdp

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type PathDef struct {
	Interaction fhir.TypeRestfulInteraction
	PathDef     []string
	Verb        string
}

// https://hl7.org/fhir/R4/http.html
var definitions = []PathDef{
	{
		Interaction: fhir.TypeRestfulInteractionRead,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionVread,
		PathDef:     []string{"[type]", "[id]", "_history", "[vid]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionUpdate,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "PUT",
	},
	{
		Interaction: fhir.TypeRestfulInteractionPatch,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "PATCH",
	},
	{
		Interaction: fhir.TypeRestfulInteractionDelete,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "DELETE",
	},
	{
		Interaction: fhir.TypeRestfulInteractionDelete,
		PathDef:     []string{"[type]?"},
		Verb:        "DELETE",
	},
	{
		Interaction: fhir.TypeRestfulInteractionCreate,
		PathDef:     []string{"[type]"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionSearchType,
		PathDef:     []string{"[type]?"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionUpdate,
		PathDef:     []string{"[type]?"},
		Verb:        "PUT",
	},
	{
		Interaction: fhir.TypeRestfulInteractionSearchType,
		PathDef:     []string{"[type]", "_search?"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionSearchSystem,
		PathDef:     []string{"?"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionCapabilities,
		PathDef:     []string{"metadata"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionTransaction,
		PathDef:     []string{},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistoryInstance,
		PathDef:     []string{"[type]", "[id]", "_history"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistoryType,
		PathDef:     []string{"[type]", "_history"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistorySystem,
		PathDef:     []string{"_history"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"$[name]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"$[name]"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "$[name]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "$[name]"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "[id]", "$[name]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "[id]", "$[name]"},
		Verb:        "POST",
	},
}

var regexId = regexp.MustCompile(`^[A-Za-z0-9\-\.]{1,64}$`)
var regexOperation = regexp.MustCompile(`^\$[a-z\-\.]+$`)

type Tokens struct {
	Interaction fhir.TypeRestfulInteraction

	ResourceType  *fhir.ResourceType
	ResourceId    string
	OperationName string
	VersionId     string
}

func parsePath(def PathDef, req HTTPRequest) (Tokens, bool) {
	var out Tokens

	if def.Verb != req.Method {
		return Tokens{}, false
	}

	// Preprocesses the path for easier manipulation
	strPath := req.Path
	if strings.HasPrefix(strPath, "/") {
		strPath = strPath[1:]
	}
	path := strings.Split(strPath, "/")

	// Early return if the path has a different length than this definition
	if len(path) != len(def.PathDef) {
		return Tokens{}, false
	}

	for idx, part := range def.PathDef {
		switch part {
		case "[type]":
			ptr, ok := parseResourceType(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.ResourceType = ptr
			continue
		case "[type]?":
			str := strings.TrimSuffix(path[idx], "?")
			ptr, ok := parseResourceType(str)
			if !ok {
				return Tokens{}, false
			}
			out.ResourceType = ptr
			continue
		case "[id]":
			ok := regexId.MatchString(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.ResourceId = path[idx]
			continue
		case "[vid]":
			ok := regexId.MatchString(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.VersionId = path[idx]
			continue
		case "$[name]":
			ok := regexOperation.MatchString(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.OperationName = strings.TrimPrefix(path[idx], "$")
			continue
		}

		if path[idx] != part {
			return Tokens{}, false
		}
	}

	return out, true
}

func parseResourceType(str string) (*fhir.ResourceType, bool) {
	var t fhir.ResourceType
	err := t.UnmarshalJSON([]byte(str))
	if err != nil {
		return nil, false
	}
	return &t, true
}

func parseRequestPath(request HTTPRequest) (Tokens, bool) {
	var tokens Tokens
	var def PathDef
	var ok bool
	for _, d := range definitions {
		tokens, ok = parsePath(d, request)
		if ok {
			def = d
			break
		}
	}

	if !ok {
		return tokens, false
	}

	tokens.Interaction = def.Interaction
	return tokens, true
}

type Params struct {
	SearchParams map[string]string
	Revinclude   []string
	Include      []string
}

var generalParams = []string{
	"_format",
	"_pretty",
	"_summary",
	"_elements",
}

var resultParams = []string{
	"_sort",
	"_count",
	"_include",
	"_revinclude",
	"_summary",
	"_total",
	"_elements",
	"_contained",
	"_containedType",
}

func groupParams(queryParams url.Values) Params {
	var params Params

	params.Include = queryParams["_include"]
	delete(queryParams, "_include")
	if params.Include == nil {
		// init to empty slice for consistency
		params.Include = []string{}
	}

	params.Revinclude = queryParams["_revinclude"]
	delete(queryParams, "_revinclude")
	if params.Revinclude == nil {
		// init to empty slice for consistency
		params.Revinclude = []string{}
	}

	for _, p := range generalParams {
		delete(queryParams, p)
	}
	for _, p := range resultParams {
		delete(queryParams, p)
	}

	// Convert remaining query params to map
	params.SearchParams = make(map[string]string)
	for key, values := range queryParams {
		// Join multiple values with comma
		params.SearchParams[key] = strings.Join(values, ",")
	}

	return params
}

func derivePatientId(tokens Tokens, queryParams url.Values) (string, error) {
	if tokens.ResourceType != nil && *tokens.ResourceType == fhir.ResourceTypePatient {
		// https://fhir.example.org/Patient/12345
		if tokens.ResourceId != "" {
			return tokens.ResourceId, nil
		}

		// https://fhir.example.org/Patient?_id=12345
		return getSingleParameter(queryParams, "_id")
	}

	// TODO: make this resource-specific
	// https://fhir.example.org/Encounter?patient=Patient/12345
	patientInParams := len(queryParams["patient"]) == 1
	if patientInParams {
		refStr := queryParams["patient"][0]
		parts := strings.Split(refStr, "/")
		return parts[len(parts)-1], nil
	}

	if len(queryParams["patient"]) > 1 {
		return "", fmt.Errorf("multiple patient parameters found")
	}

	return "", nil
}

func derivePatientBSN(tokens Tokens, rawParams url.Values) (string, error) {
	// TODO: support other resource types if needed
	if tokens.ResourceType == nil || *tokens.ResourceType != fhir.ResourceTypePatient {
		return "", nil
	}
	identifier, err := getSingleParameter(rawParams, "identifier")
	if err != nil {
		return "", err
	}
	if identifier == "" {
		return "", nil
	}
	parts := strings.Split(identifier, "|")
	if len(parts) != 2 {
		return "", errors.New("expected identifier parameter in format 'system|value'")
	}
	if parts[0] != coding.BSNNamingSystem {
		return "", fmt.Errorf("expected identifier system to be '%s', found '%s'", coding.BSNNamingSystem, parts[0])
	}
	if parts[1] == "" {
		return "", errors.New("identifier value is empty")
	}
	return parts[1], nil
}

func getSingleParameter(params url.Values, name string) (string, error) {
	values := params[name]
	if len(values) == 0 {
		return "", nil
	} else if len(values) > 1 {
		return "", fmt.Errorf("multiple %s parameters found", name)
	}
	value := values[0]
	if strings.Count(value, ",") != 0 {
		return "", fmt.Errorf("expected 1 value in %s parameter, found multiple", name)
	}
	return value, nil
}

func NewPolicyInput(request PDPRequest) (PolicyInput, PolicyResult) {
	var policyInput PolicyInput

	policyInput.Subject = request.Input.Subject
	policyInput.Action.Properties.Request = request.Input.Request
	policyInput.Context.DataHolderOrganizationId = request.Input.Context.DataHolderOrganizationId
	policyInput.Context.DataHolderFacilityType = request.Input.Context.DataHolderFacilityType
	policyInput.Context.PatientBSN = request.Input.Context.PatientBSN

	contentType := request.Input.Request.Header.Get("Content-Type")
	policyInput.Action.Properties.ContentType = contentType

	tokens, ok := parseRequestPath(request.Input.Request)
	if !ok {
		// This is not a FHIR request
		return policyInput, Allow()
	}

	if tokens.ResourceType != nil {
		policyInput.Resource.Type = *tokens.ResourceType
		if tokens.ResourceId != "" {
			policyInput.Resource.Properties.ResourceId = tokens.ResourceId
		}
		if tokens.VersionId != "" {
			policyInput.Resource.Properties.VersionId = tokens.VersionId
		}
	}

	policyInput.Action.Properties.InteractionType = tokens.Interaction

	if tokens.OperationName != "" {
		policyInput.Action.Properties.Operation = &tokens.OperationName
	}

	var rawParams url.Values
	hasFormData := contentType == "application/x-www-form-urlencoded"
	interWithBody := []fhir.TypeRestfulInteraction{
		fhir.TypeRestfulInteractionSearchType,
		fhir.TypeRestfulInteractionOperation,
	}
	paramsInBody :=
		slices.Contains(interWithBody, tokens.Interaction) &&
			hasFormData

	if paramsInBody {
		values, err := url.ParseQuery(request.Input.Request.Body)
		if err != nil {
			reason := ResultReason{
				Code:        TypeResultCodeUnexpectedInput,
				Description: "Could not parse form encoded data",
			}
			return PolicyInput{}, Deny(reason)
		}
		rawParams = values
	} else {
		rawParams = request.Input.Request.QueryParams
	}

	params := groupParams(rawParams)
	policyInput.Action.Properties.Include = params.Include
	policyInput.Action.Properties.Revinclude = params.Revinclude
	policyInput.Action.Properties.SearchParams = params.SearchParams

	// Read patient resource ID from request
	patientId, err := derivePatientId(tokens, rawParams)
	if err != nil {
		return PolicyInput{}, Deny(ResultReason{
			Code:        TypeResultCodeUnexpectedInput,
			Description: "patient_id: " + err.Error(),
		})
	}
	policyInput.Context.PatientID = patientId

	// Read patient BSN from request
	if policyInput.Context.PatientBSN == "" {
		patientBSN, err := derivePatientBSN(tokens, rawParams)
		if err != nil {
			return PolicyInput{}, Deny(ResultReason{
				Code:        TypeResultCodeUnexpectedInput,
				Description: "patient_bsn: " + err.Error(),
			})
		}
		policyInput.Context.PatientBSN = patientBSN
	}

	return policyInput, Allow()
}
