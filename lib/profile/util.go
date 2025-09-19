package profile

import (
	"slices"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Set(meta *fhir.Meta, profileURL string) *fhir.Meta {
	if meta == nil {
		meta = &fhir.Meta{}
	}
	if !slices.Contains(meta.Profile, profileURL) {
		meta.Profile = append(meta.Profile, profileURL)
	}
	return meta
}
