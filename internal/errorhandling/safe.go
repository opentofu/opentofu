package errorhandling

import "fmt"

// safe2 runs the specified function and returns its result value or returned error. If a panic occurs, it returns the
// panic as an error.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func safe2[TValue any](f func() (TValue, error)) (result TValue, err error) {
	defer func() {
		var ok bool
		e := recover()
		if e == nil {
			return
		}
		if err, ok = e.(error); !ok {
			// In case the panic is not an error
			err = fmt.Errorf("%v", e)
		}
	}()
	return f()
}

// Safe2 runs the specified function and returns its result value or returned error. If a panic occurs, it returns the
// panic as an error. Any errors returned are passed through the passed wrap function.
//
// Note: this is equivalent to a try-catch and you should probably not use it. Only use if you need to handle
// panics from third party libraries or from Golang itself.
func Safe2[TValue any](f func() (TValue, error), wrapError func(err error) error) (result TValue, err error) {
	value, err := safe2(f)
	if err != nil {
		return value, wrapError(err)
	}
	return value, nil
}
