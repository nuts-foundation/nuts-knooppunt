package config

import (
	"net/url"
)

const FHIR_SYSTEM_URA = "http://fhir.nl/fhir/NamingSystem/ura"
const FHIR_CONNECTION_TYPE_SYTEM = "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"

// Directory represents a mCSD directory. It contains configuration values about the name, URL, and properties of the directory.
type Directory struct {
	Name string  `json:"name"`
	Url  url.URL `json:"url"`
	// AuthoritativeOf is a list of properties indicated by FHIRPath that this directory is authoritative for.
	AuthoritativeOf []string `json:"authorative_of,omitempty"` // List of resource types that this directory is authoritative for
}

type Config struct {
	// LocalDirectory contains the directory where the client stores its data.
	LocalDirectory Directory `json:"local_directory"`
	// MasterDirectory contains the authentic source directory where the organizations and there endpoints are stored
	MasterDirectory Directory `json:"master_directory"`
	// AuthenticDirectories contains the list of directories that are considered authentic for specific properties of resources
	AuthenticDirectories []Directory `json:"authentic_directories"`
	// ResourceIdentifiers contains for each resource the type of the common agreed upon identifier. This identifier is used to uniquely match resources across directories.
	// The key is the resource type (e.g., "organization", "endpoint") and the value is the identifier type system (e.g., "http://fhir.nl/fhir/NamingSystem/ura").
	// This is used to ensure that resources can be identified consistently across different directories.
	ResouceIdentifiers map[string]string `json:"resource_identifiers,omitempty"`
}

// NewConfig creates a new Config with default values.
// These values can be used for development and testing purposes.
func NewExampleConfig() *Config {
	return &Config{
		ResouceIdentifiers: map[string]string{
			"organization": "http://fhir.nl/fhir/NamingSystem/ura",
		},
		LocalDirectory: Directory{
			Name: "local",
			Url:  url.URL{Scheme: "http", Host: "localhost:8080", Path: "/fhir/local/"},
		},
		MasterDirectory: Directory{
			Name: "master",
			Url:  url.URL{Scheme: "http", Host: "localhost:8080", Path: "/fhir/LRZA/"},
			AuthoritativeOf: []string{
				// Authoritative of the URA identifier for organizations
				"Organization.identifiers.where(type='http://fhir.nl/fhir/NamingSystem/ura')",
				// Authoritative of the mCSD-directory connection type for endpoints
				"Endpoint.connectionType.coding.where(system='http://fhir.nl/fhir/NamingSystem/endpoint-connection-type').where(code='mCSD-directory')",
			},
		},
		// AuthenticDirectories: []Directory{
		// 	{
		// 		Name: "authentic1",
		// 		Url:  url.URL{Scheme: "https", Host: "authentic1.com", Path: "/data"},
		// 	},
		// 	{
		// 		Name: "authentic2",
		// 		Url:  url.URL{Scheme: "https", Host: "authentic2.com", Path: "/data"},
		// 	},
		// },
	}
}
