package apperr

import "errors"

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string       { return e.Code + ": " + e.Message }
func New(code, message string) error { return &Error{code, message} }
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "INTERNAL_ERROR"
}
