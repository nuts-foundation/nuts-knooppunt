package fhirutil

import (
	"encoding/json"
	"fmt"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// VisitBundleResources iterates over all entries in the bundle,
// unmarshals the entry's resource to the specified ResType and calls the visitor function.
func VisitBundleResources[ResType any](bundle *fhir.Bundle, visitor func(resource *ResType) error) error {
	for i, entry := range bundle.Entry {
		if entry.Resource == nil {
			continue
		}
		var res ResType
		if err := json.Unmarshal(entry.Resource, &res); err != nil {
			return fmt.Errorf("unmarshal bundle entry resource into %T: %w", res, err)
		}
		if err := visitor(&res); err != nil {
			return fmt.Errorf("visit bundle entry resource %T: %w", res, err)
		}
		data, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("remarshal bundle entry resource %T: %w", res, err)
		}
		entry.Resource = data
		bundle.Entry[i] = entry
	}
	return nil
}
