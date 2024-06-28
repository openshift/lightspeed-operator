package e2e

func Ptr[T any](v T) *T { return &v }
