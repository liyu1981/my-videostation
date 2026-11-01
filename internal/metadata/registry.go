package metadata

import (
	"context"
	"fmt"
	"sync"
)

// Registry maps provider names to factories. The first registered provider is the
// default for automatic enrichment and provider-less requests.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	order     []string
}

func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

func (r *Registry) Register(name string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.factories[name]; !ok {
		r.order = append(r.order, name)
	}
	r.factories[name] = f
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

func (r *Registry) DefaultName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.order) == 0 {
		return ""
	}
	return r.order[0]
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

func (r *Registry) Get(ctx context.Context, name string) (Provider, error) {
	r.mu.RLock()
	if name == "" {
		name = ""
		if len(r.order) > 0 {
			name = r.order[0]
		}
	}
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok || f == nil {
		return nil, fmt.Errorf("unknown metadata provider %q", name)
	}
	return f(ctx)
}
