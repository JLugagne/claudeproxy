package domain

import (
	"errors"
	"sync"
	"testing"
)

func TestRouterResolveMemoizes(t *testing.T) {
	route, err := NewGlobRoute("claude-*", "anthropic", "")
	if err != nil {
		t.Fatalf("NewGlobRoute: %v", err)
	}
	router := NewRouter([]Route{route}, "openrouter")

	first, err := router.Resolve("claude-opus-4")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := router.Resolve("claude-opus-4")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if first != second {
		t.Fatalf("cached decision differs: %+v vs %+v", first, second)
	}
	if _, ok := router.cache.Load(Model("claude-opus-4")); !ok {
		t.Fatal("expected a matching model to be memoized")
	}
}

func TestRouterResolveMemoizesError(t *testing.T) {
	router := NewRouter(nil, "")

	if _, err := router.Resolve("unknown-model"); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("Resolve error = %v, want ErrNoRoute", err)
	}
	if _, ok := router.cache.Load(Model("unknown-model")); !ok {
		t.Fatal("expected the ErrNoRoute outcome to be memoized")
	}
}

func TestRouterResolveConcurrent(t *testing.T) {
	route, err := NewGlobRoute("claude-*", "anthropic", "")
	if err != nil {
		t.Fatalf("NewGlobRoute: %v", err)
	}
	router := NewRouter([]Route{route}, "openrouter")

	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, err := router.Resolve("claude-opus-4")
			if err != nil {
				t.Errorf("Resolve: %v", err)
				return
			}
			if decision.Provider != "anthropic" {
				t.Errorf("provider = %q, want anthropic", decision.Provider)
			}
		}()
	}
	wg.Wait()
}
