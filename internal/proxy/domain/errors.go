package domain

import "errors"

var (
	// ErrInvalidRoute is returned when a route definition cannot be compiled.
	ErrInvalidRoute = errors.New("invalid route")
	// ErrNoRoute is returned when no route matches and no fallback provider is set.
	ErrNoRoute = errors.New("no route for model")
)
