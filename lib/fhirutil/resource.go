package fhirutil

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Resource is a FHIR resource as generic JSON. Unlike a typed model, it keeps every element of the
// resource - including ones the code doesn't know about, such as extensions - when it is changed and
// written back, so it suits code that adds to a resource it otherwise passes on unchanged.
type Resource map[string]any

// ParseResource parses a JSON object as a Resource.
func ParseResource(data []byte) (Resource, error) {
	var result Resource
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("not a valid FHIR resource: %w", err)
	}
	if result == nil {
		return nil, errors.New("not a valid FHIR resource")
	}
	return result, nil
}

// ParseResourceOfType parses a JSON object as a Resource of the given type, as received in a request
// for that type. If id is set (as for an update), the resource must have that id or none, and gets id.
func ParseResourceOfType(data []byte, resourceType, id string) (Resource, error) {
	res, err := ParseResource(data)
	if err != nil {
		return nil, err
	}
	if res.ResourceType() != resourceType {
		return nil, fmt.Errorf("resourceType %q does not match the expected type (%s)", res.ResourceType(), resourceType)
	}
	if id != "" {
		if res.ID() != "" && res.ID() != id {
			return nil, fmt.Errorf("resource id %q does not match the expected id (%s)", res.ID(), id)
		}
		res["id"] = id
	}
	return res, nil
}

func (r Resource) ResourceType() string {
	value, _ := r["resourceType"].(string)
	return value
}

func (r Resource) ID() string {
	value, _ := r["id"].(string)
	return value
}

// VersionID is meta.versionId, or "" if absent.
func (r Resource) VersionID() string {
	meta, _ := r["meta"].(map[string]any)
	value, _ := meta["versionId"].(string)
	return value
}

// Reference is the relative reference to the resource, e.g. "Organization/123".
func (r Resource) Reference() string {
	return r.ResourceType() + "/" + r.ID()
}

// Identifiers returns the resource's identifiers. They are the resource's own maps, so a change to
// one changes the resource.
func (r Resource) Identifiers() []map[string]any {
	items, _ := r["identifier"].([]any)
	var result []map[string]any
	for _, item := range items {
		if identifier, ok := item.(map[string]any); ok {
			result = append(result, identifier)
		}
	}
	return result
}

// Clone returns a deep copy, so a resource can be both kept and modified.
func (r Resource) Clone() Resource {
	data, _ := json.Marshal(r)
	var result Resource
	_ = json.Unmarshal(data, &result)
	return result
}

// ToJSONMap converts a typed FHIR element (e.g. a fhir.Identifier) to the generic JSON form a
// Resource holds.
func ToJSONMap(value any) map[string]any {
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}
