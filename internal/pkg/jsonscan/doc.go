// Package jsonscan locates top-level members of a JSON object without decoding
// the whole document, so the proxy can read and rewrite the "model" field of a
// multi-megabyte request body in a single forward pass with no allocation.
//
// It is a leaf utility outside the hexagonal layering: it imports nothing from
// the project and must never import a bounded context. It is not a validator;
// malformed input that happens to expose the requested key may still match,
// the upstream remains responsible for rejecting invalid bodies.
package jsonscan
