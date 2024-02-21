package golang

import "fmt"

// Safe runs the specified function. If a panic occurs, it returns the panic as an error.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe(f func()) (err error) {
	defer func() {
		var ok bool
		e := recover()
		if err, ok = e.(error); !ok {
			// In case the panic is not an error
			err = fmt.Errorf("%v", e)
		}
	}()
	f()
	return
}

// Safe1 runs the specified function and returns its result value. If a panic occurs, it returns the panic as an error.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe1[TValue any](f func() TValue) (result TValue, err error) {
	defer func() {
		var ok bool
		e := recover()
		if err, ok = e.(error); !ok {
			// In case the panic is not an error
			err = fmt.Errorf("%v", e)
		}
	}()
	return f(), nil
}

// Safe1e runs the specified function and returns any returned error. If a panic occurs, it returns the panic as
// the error.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe1e(f func() error) (err error) {
	defer func() {
		var ok bool
		e := recover()
		if err, ok = e.(error); !ok {
			// In case the panic is not an error
			err = fmt.Errorf("%v", e)
		}
	}()
	return f()
}

// Safe1w runs the specified function and returns any returned error. If a panic occurs, it returns the
// panic as an error. Any errors returned are passed through the passed wrap function.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe1w(f func() error, wrap func(err error) error) error {
	err := Safe1e(f)
	if err != nil {
		return wrap(err)
	}
	return nil
}

// Safe2 runs the specified function and returns its return values. If a panic occurs, it returns the panic as an error.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe2[TValue1 any, TValue2 any](f func() (TValue1, TValue2)) (result1 TValue1, result2 TValue2, err error) {
	defer func() {
		var ok bool
		e := recover()
		if err, ok = e.(error); !ok {
			// In case the panic is not an error
			err = fmt.Errorf("%v", e)
		}
	}()
	result1, result2 = f()
	return result1, result2, nil
}

// Safe2e runs the specified function and returns its result value or returned error. If a panic occurs, it returns the
// panic as an error.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe2e[TValue any](f func() (TValue, error)) (result TValue, err error) {
	defer func() {
		var ok bool
		e := recover()
		if err, ok = e.(error); !ok {
			// In case the panic is not an error
			err = fmt.Errorf("%v", e)
		}
	}()
	return f()
}

// Safe2w runs the specified function and returns its result value or returned error. If a panic occurs, it returns the
// panic as an error. Any errors returned are passed through the passed wrap function.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe2w[TValue any](f func() (TValue, error), wrap func(err error) error) (result TValue, err error) {
	value, err := Safe2e(f)
	if err != nil {
		return value, wrap(err)
	}
	return value, nil
}
