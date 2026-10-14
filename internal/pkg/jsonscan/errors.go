package jsonscan

import "errors"

var (
	// ErrNotFound is returned when the object has no member with the requested key.
	ErrNotFound = errors.New("jsonscan: key not found")
	// ErrNotString is returned when the requested member exists but is not a JSON string.
	ErrNotString = errors.New("jsonscan: value is not a string")
	// ErrMalformed is returned when the input is not a JSON object or ends prematurely.
	ErrMalformed = errors.New("jsonscan: malformed JSON")
)
