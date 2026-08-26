package to

import (
	"encoding/json"
)

func EmptyString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func Ptr[T any](v T) *T {
	return &v
}

func JSONMap[T any](val T) (map[string]any, error) {
	m := make(map[string]any)
	data, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// JSONConvert converts in to TOut by marshaling to JSON and unmarshaling into TOut. Useful
// between types with identical JSON shapes but different Go representations, e.g. between the
// generated OpenAPI client's opaque map-shaped types and this codebase's typed structs.
func JSONConvert[TOut any](in any) (TOut, error) {
	var out TOut
	data, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}
