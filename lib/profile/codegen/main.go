package main

import (
	"io"
	"net/http"
	"os"
	"strings"
)

// main downloads the profile structure definitions and saves them in the profiles folder
func main() {
	profileDefintions := []string{
		"https://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition-nl-gf-organization.json",
	}
	for _, definitionURL := range profileDefintions {
		httpResponse, err := http.Get(definitionURL)
		if err != nil {
			panic(err)
		}
		if httpResponse.StatusCode != http.StatusOK {
			panic("failed to download profile: " + definitionURL)
		}
		data, err := io.ReadAll(httpResponse.Body)
		if err != nil {
			panic(err)
		}
		// fileName is last path part of URL
		urlParts := strings.Split(definitionURL, "/")
		fileName := urlParts[len(urlParts)-1]
		err = os.WriteFile(fileName, data, 0644)
		if err != nil {
			panic(err)
		}
	}
}
