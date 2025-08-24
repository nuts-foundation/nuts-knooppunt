package vectors

func toPtr[T any](v T) *T {
	return &v
}
