package golang

// Must throws a panic if the passed error is not nil. You can use this where you do not expect an error to happen, but
// would like to make sure that any potential errors are caught.
//
// Note: you should only use this in test cases or when you are absolutely sure no error can happen under normal
// circumstances as it produces a hideous panic in the user's face otherwise. Seriously, don't use it outside test
// cases.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// Must2 throws a panic if the passed error is not nil and returns the passed value otherwise. You can use this where
// you do not expect an error to happen, but would like to make sure that any potential errors are caught.
//
// Note: you should only use this in test cases or when you are absolutely sure no error can happen under normal
// circumstances as it produces a hideous panic in the user's face otherwise. Seriously, don't use it outside test
// cases.
func Must2[TValue any](value TValue, err error) TValue {
	if err != nil {
		panic(err)
	}
	return value
}

// Must3 throws a panic if the passed error is not nil and returns the passed values otherwise. You can use this where
// you do not expect an error to happen, but would like to make sure that any potential errors are caught.
//
// Note: you should only use this in test cases or when you are absolutely sure no error can happen under normal
// circumstances as it produces a hideous panic in the user's face otherwise. Seriously, don't use it outside test
// cases.
func Must3[TValue1 any, TValue2 any](value1 TValue1, value2 TValue2, err error) (TValue1, TValue2) {
	if err != nil {
		panic(err)
	}
	return value1, value2
}
