package llm

import (
	"context"
	"sync/atomic"
)

// SwappableProvider wraps an LLMProvider behind an atomic pointer, allowing
// the active provider to be replaced at runtime without restarting.
// All components can hold a *SwappableProvider (which implements LLMProvider),
// and a single Swap call transparently routes all future calls to the new provider.
type SwappableProvider struct {
	inner atomic.Pointer[LLMProvider]
}

// NewSwappable creates a SwappableProvider backed by initial.
func NewSwappable(initial LLMProvider) *SwappableProvider {
	s := &SwappableProvider{}
	s.inner.Store(&initial)
	return s
}

// Swap atomically replaces the underlying provider. In-flight calls complete
// against the old provider; new calls use the replacement.
func (s *SwappableProvider) Swap(p LLMProvider) {
	s.inner.Store(&p)
}

// Current returns the active underlying provider.
func (s *SwappableProvider) Current() LLMProvider {
	return *s.inner.Load()
}

func (s *SwappableProvider) Model() string {
	return s.Current().Model()
}

func (s *SwappableProvider) Complete(ctx context.Context, req CompletionRequest) (*Response, error) {
	return s.Current().Complete(ctx, req)
}

func (s *SwappableProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error) {
	return s.Current().Stream(ctx, req)
}

func (s *SwappableProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return s.Current().Embed(ctx, text)
}
