package formdata

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"

	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var keyExp regexp.Regexp

func init() {
	exp, err := regexp.Compile(`(\w+)\[(\d*)\]\[(\w+)\]`)
	if err != nil {
		log.Error().Err(err).Msg("could not parse regular expression")
		return
	}
	keyExp = *exp
}

func ParseStructs[S any](postform url.Values, key string) ([]S, error) {
	var item S
	valueOfS := reflect.ValueOf(item)
	if valueOfS.Kind() != reflect.Struct {
		return []S{item}, errors.New("input type not a struct")
	}

	typeOfS := valueOfS.Type()
	for i := 0; i < valueOfS.NumField(); i++ {
		f := valueOfS.Field(i)
		fmt.Printf("%d: %s %s = %v\n", i,
			typeOfS.Field(i).Name, f.Type(), f.Interface())
	}

	return []S{item}, errors.New("not implemented")
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
			log.Warn().Msg(fmt.Sprintf("conflicting values found for key: %s", fk))
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

func filterEmpty(multiStrings []string) []string {
	out := make([]string, 0, len(multiStrings))
	for _, str := range multiStrings {
		if str != "" {
			out = append(out, str)
		}
	}
	return out
}
