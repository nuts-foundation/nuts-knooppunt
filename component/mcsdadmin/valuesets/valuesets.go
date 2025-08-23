package valuesets

import (
	"embed"
	"encoding/json"

	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed *.json
var setsFS embed.FS

func CodingsFrom(setId string) (out []fhir.Coding, err error) {
	filename := setId + ".json"
	bytes, err := setsFS.ReadFile(filename)
	if err != nil {
		log.Warn().Err(err).Msg("Could not load file with values in set")
		return out, err
	}

	var codings []fhir.Coding
	err = json.Unmarshal(bytes, &codings)
	if err != nil {
		log.Warn().Err(err).Msg("Invalid values in file")
		return out, err
	}

	return codings, nil
}
