package retry

import "errors"

type RecoverableError interface {
	error
	IsRecoverable() bool
}

func IsRecoverable(err error) bool {
	if err == nil {
		return false
	}
	var recoverable RecoverableError
	return errors.As(err, &recoverable) && recoverable.IsRecoverable()
}

type recoverableError struct {
	err error
}

func (e *recoverableError) Error() string {
	return e.err.Error()
}

func (e *recoverableError) IsRecoverable() bool {
	return true
}

func (e *recoverableError) Unwrap() error {
	return e.err
}

func NewRecoverableError(err error) *recoverableError {
	return &recoverableError{err: err}
}
