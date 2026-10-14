package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Route sends the models matching its pattern to one provider, optionally
// renaming the model. Routes are immutable and safe for concurrent use.
type Route struct {
	pattern  *regexp.Regexp
	provider ProviderName
	rewrite  string
}

// NewGlobRoute builds a route from a glob where '*' matches any sequence
// (including '/') and '?' matches one character; the whole model name must
// match. Each wildcard is a capture group usable in rewrite as $1, $2, ...
// An empty rewrite keeps the requested model. It returns ErrInvalidRoute.
func NewGlobRoute(glob string, provider ProviderName, rewrite string) (Route, error) {
	var expr strings.Builder
	expr.WriteByte('^')
	for _, r := range glob {
		switch r {
		case '*':
			expr.WriteString("(.*)")
		case '?':
			expr.WriteString("(.)")
		default:
			expr.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	expr.WriteByte('$')
	return newRoute(expr.String(), provider, rewrite)
}

// NewRegexRoute builds a route from an RE2 expression, unanchored unless the
// expression anchors itself. rewrite follows regexp.Expand syntax ($1, ${name}).
// An empty rewrite keeps the requested model. It returns ErrInvalidRoute.
func NewRegexRoute(expr string, provider ProviderName, rewrite string) (Route, error) {
	return newRoute(expr, provider, rewrite)
}

func newRoute(expr string, provider ProviderName, rewrite string) (Route, error) {
	if provider == "" {
		return Route{}, errors.Join(ErrInvalidRoute, errors.New("provider is required"))
	}
	pattern, err := regexp.Compile(expr)
	if err != nil {
		return Route{}, errors.Join(ErrInvalidRoute, fmt.Errorf("pattern %q", expr), err)
	}
	return Route{pattern: pattern, provider: provider, rewrite: rewrite}, nil
}

func (r Route) resolve(model Model) (Model, bool) {
	match := r.pattern.FindStringSubmatchIndex(string(model))
	if match == nil {
		return "", false
	}
	if r.rewrite == "" {
		return model, true
	}
	return Model(r.pattern.ExpandString(nil, r.rewrite, string(model), match)), true
}
