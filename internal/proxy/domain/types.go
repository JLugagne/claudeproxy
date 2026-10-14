// Package domain holds the routing rules of the proxy: which provider serves a
// given model name and under which upstream model name. It is pure: no I/O,
// no transport, no configuration format.
package domain

// ProviderName identifies an upstream provider declared in the configuration.
type ProviderName string

// Model is a model identifier as sent by the client in the request body. The
// empty Model means the request carries no model (e.g. GET /v1/models).
type Model string

// Decision is the outcome of routing one request.
type Decision struct {
	Provider ProviderName
	// Model is the model name to send upstream.
	Model Model
	// Requested is the model name sent by the client.
	Requested Model
}

// Rewritten reports whether the upstream model differs from the requested one,
// meaning the request body must be modified before forwarding.
func (d Decision) Rewritten() bool {
	return d.Model != d.Requested
}
