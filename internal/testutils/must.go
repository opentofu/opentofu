package testutils

// Must turns an error into a panic. Use for tests only.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// Must2 panics if err is an error, otherwise it returns the value.
func Must2[T any](value T, err error) T {
	Must(err)
	return value
}
