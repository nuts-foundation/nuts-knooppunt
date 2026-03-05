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
