package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Consolidator struct {
	hashSpace uuid.UUID // UUID used to ensure unique resource IDs
}

func NewConsolidator() *Consolidator {
	return &Consolidator{
		hashSpace: hashSpace,
	}
}

// hashSpace is a UUID that is used to ensure that the generated UUIDs for resources are unique across different instances of the application.
// value must be configured and different for each instance of the application to avoid collisions.
var hashSpace = uuid.MustParse("f416463a-8aa2-47b6-9e74-cf5e64e56853")

func (c *Consolidator) deterministicUUID(id string) string {
	// Generate a UUID based on the input ID and a fixed namespace to ensure uniqueness
	return "urn:uuid:" + uuid.NewSHA1(c.hashSpace, []byte(id)).String()
}

func (c *Consolidator) FeedFromUpdateBundle(updateBundle *fhir.Bundle) (*fhir.Bundle, error) {
	txBundle := fhir.Bundle{
		Type: fhir.BundleTypeTransaction,
	}

	// First pass: give each resource a new UUID and update the ID mapping
	for _, entry := range updateBundle.Entry {

		unknownEntry := struct {
			Id           *string `bson:"id,omitempty" json:"id,omitempty"`
			ResourceType string  `json:"resourceType"`
		}{}

		if err := json.Unmarshal(entry.Resource, &unknownEntry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource: %w", err)
		}
		fmt.Println("Processing resource of type:", unknownEntry.ResourceType)
		fmt.Println("Resource ID:", *unknownEntry.Id)

		newId := c.deterministicUUID(*unknownEntry.Id)

		switch unknownEntry.ResourceType {
		case "Organization":
			org := fhir.Organization{}
			json.Unmarshal(entry.Resource, &org)
			org.Id = &newId
			for _, ep := range org.Endpoint {
				// Update the Endpoint reference in Organization
				if ep.Reference != nil {
					oldId := strings.Split(*ep.Reference, "/")[1] // Extract the old ID from the reference
					*ep.Reference = c.deterministicUUID(oldId)    // Update to new ID
				}
			}
			// only create the Organization if it does not already exist. Matching by the identifier
			condition := "Organization?identifier=http://fhir.nl/fhir/NamingSystem/ura|" + *org.Identifier[0].Value

			entry.Request = &fhir.BundleEntryRequest{
				Method:      fhir.HTTPVerbPOST,
				Url:         "Organization",
				IfNoneExist: &condition,
			}
			entry.Resource, _ = json.Marshal(org) // Re-encode the modified resource
		case "Endpoint":
			endpoint := fhir.Endpoint{}
			json.Unmarshal(entry.Resource, &endpoint)
			endpoint.Id = &newId
			oldOrgId := strings.Split(*endpoint.ManagingOrganization.Reference, "/")[1] // Extract the old ID from the reference
			newOrgId := c.deterministicUUID(oldOrgId)                                   // Generate new ID based on old ID
			endpoint.ManagingOrganization.Reference = &newOrgId
			entry.Resource, _ = json.Marshal(endpoint) // Re-encode the modified resource
		}

		entry.FullUrl = nil
		entry.Response = nil
		txBundle.Entry = append(txBundle.Entry, entry)
	}

	return &txBundle, nil
}
