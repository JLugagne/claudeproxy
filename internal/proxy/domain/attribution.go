package domain

import "context"

// Attribution identifies the client project and session a request belongs to.
// It is observability metadata: routing never depends on it.
type Attribution struct {
	Project string
	Session string
}

// Unknown is the project or session value used when the client did not identify
// itself.
const Unknown = "unknown"

// Normalized returns a with each empty field replaced by Unknown.
func (a Attribution) Normalized() Attribution {
	if a.Project == "" {
		a.Project = Unknown
	}
	if a.Session == "" {
		a.Session = Unknown
	}
	return a
}

type attributionKey struct{}

// WithAttribution returns a context carrying a.
func WithAttribution(ctx context.Context, a Attribution) context.Context {
	return context.WithValue(ctx, attributionKey{}, a)
}

// AttributionFrom returns the Attribution carried by ctx, or the zero value.
func AttributionFrom(ctx context.Context) Attribution {
	a, _ := ctx.Value(attributionKey{}).(Attribution)
	return a
}
