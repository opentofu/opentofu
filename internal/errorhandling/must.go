package errorhandling

// Must converts an error into a panic.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// Must2 converts an error into a panic, returning a value if no error happened.
func Must2[T any](value T, err error) T {
	Must(err)
	return value
}
