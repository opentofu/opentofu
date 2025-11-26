package command

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ExitCodeBaseError                = fmt.Errorf("exit with code")
	UnexpectedErrorReturnedByCommand = fmt.Errorf("unexpected error returned by command")
)

const (
	OkExitCode           = 0
	DefaultErrorExitCode = 1
)

type ExitCodeError struct {
	Cause    error
	ExitCode int
}

func (ece *ExitCodeError) Error() string {
	return fmt.Errorf("%w: %d", ExitCodeBaseError, ece.ExitCode).Error()
}

func ExtractExitCode(err error) (exitCode int, rootCause error) {
	if err == nil {
		return OkExitCode, nil
	}
	var expected *ExitCodeError
	if !errors.As(err, &expected) {
		return DefaultErrorExitCode, fmt.Errorf("%w: %s", UnexpectedErrorReturnedByCommand, reflect.TypeOf(err))
	}
	return expected.ExitCode, expected.Cause
}
