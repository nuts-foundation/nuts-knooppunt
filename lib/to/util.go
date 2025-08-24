package to

func EmptyString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func Ptr[T any](v T) *T {
	return &v
}
