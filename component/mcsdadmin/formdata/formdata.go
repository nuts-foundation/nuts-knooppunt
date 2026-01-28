package formdata

import (
	"log/slog"
	"net/url"
	"regexp"

	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var keyExp regexp.Regexp

func init() {
	exp, err := regexp.Compile(`(\w+)\[(\d*)\]\[(\w+)\]`)
	if err != nil {
		slog.Error("could not parse regular expression", logging.Error(err))
		return
	}
	keyExp = *exp
}

func ParseMaps(postform url.Values, fieldName string) []map[string]string {
	type index = string
	type key = string
	type value = string
	var partials = map[index]map[key]value{}

	// Iterate over relevant keys and pull out the relevant data into partials
	for fk, val := range postform {
		matches := keyExp.FindStringSubmatch(fk)
		if len(matches) < 4 {
			continue
		}
		fieldNameMatch := matches[1]
		indexMatch := matches[2]
		propKeyMatch := matches[3]

		if fieldNameMatch != fieldName {
			continue
		}

		// Find if we already have some data from other keys...
		// ... if not create a new map
		partial, ok := partials[indexMatch]
		if !ok {
			partial = map[key]value{}
			partials[indexMatch] = partial
		}

		if len(val) > 1 {
			slog.Warn("conflicting values found for key", slog.String("key", fk))
		}
		partial[propKeyMatch] = val[0]
	}

	// Now let's construct the return value
	partialsLen := len(partials)
	out := make([]map[key]value, 0, partialsLen)
	for _, part := range partials {
		out = append(out, part)
	}
	return out
}

func CodablesFromForm(postform url.Values, set []fhir.Coding, key string) ([]fhir.CodeableConcept, bool) {
	nonEmpty := filterEmpty(postform[key])
	return valuesets.CodablesFrom(set, nonEmpty)
}

// CodablesFromFormWithCustom handles codables from form, including custom "other" option
// Supports indexed keys like "payload-type[0]", "payload-type[1]" with corresponding custom fields
func CodablesFromFormWithCustom(postform url.Values, set []fhir.Coding, key string) ([]fhir.CodeableConcept, bool) {
	codables := make([]fhir.CodeableConcept, 0)
	allOk := true

	// Pattern to match indexed keys: "key[n]"
	indexedKeyPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `\[(\d+)\]$`)

	// Collect all indexed keys
	var indexedKeys []string
	for formKey := range postform {
		if indexedKeyPattern.MatchString(formKey) {
			indexedKeys = append(indexedKeys, formKey)
		}
	}

	// Process each indexed key
	for _, indexedKey := range indexedKeys {
		matches := indexedKeyPattern.FindStringSubmatch(indexedKey)
		if len(matches) < 2 {
			continue
		}
		index := matches[1]

		values := filterEmpty(postform[indexedKey])
		for _, code := range values {
			if code == "other" {
				// Handle custom coding with indexed fields
				customSystemKey := "custom-system[" + index + "]"
				customCodeKey := "custom-code[" + index + "]"
				customDisplayKey := "custom-display[" + index + "]"

				customSystem := postform.Get(customSystemKey)
				customCode := postform.Get(customCodeKey)
				customDisplay := postform.Get(customDisplayKey)

				if customSystem == "" || customCode == "" {
					allOk = false
					continue
				}

				coding := fhir.Coding{
					System: &customSystem,
					Code:   &customCode,
				}
				if customDisplay != "" {
					coding.Display = &customDisplay
				}

				codable := fhir.CodeableConcept{
					Coding: []fhir.Coding{coding},
				}
				if customDisplay != "" {
					codable.Text = &customDisplay
				}
				codables = append(codables, codable)
			} else {
				// Handle standard coding from valueset
				codable, ok := valuesets.CodableFrom(set, code)
				if !ok {
					allOk = false
				} else {
					codables = append(codables, codable)
				}
			}
		}
	}

	return codables, allOk
}

func filterEmpty(multiStrings []string) []string {
	out := make([]string, 0, len(multiStrings))
	for _, str := range multiStrings {
		if str != "" {
			out = append(out, str)
		}
	}
	return out
}
