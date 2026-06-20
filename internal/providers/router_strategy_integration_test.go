package providers

import (
	"context"
	"testing"
	"time"

	router "gomodel/internal/router"

	"gomodel/internal/core"
)

// statsMockProvider is a mockProvider that also implements core.ProviderStats,
// letting intelligent routing observe latency/error/circuit signals.
type statsMockProvider struct {
	mockProvider
	p50          time.Duration
	p99          time.Duration
	errorRate    float64
	circuitState string
}

func (s *statsMockProvider) P50Latency() time.Duration { return s.p50 }
func (s *statsMockProvider) P99Latency() time.Duration { return s.p99 }
func (s *statsMockProvider) ErrorRate() float64        { return s.errorRate }
func (s *statsMockProvider) CircuitState() string {
	if s.circuitState == "" {
		return "closed"
	}
	return s.circuitState
}

func floatPtr(v float64) *float64 { return &v }

// registerModelWithMetadata adds a model entry carrying pricing metadata so
// cost-aware strategies have data to score.
func registerModelWithMetadata(t *testing.T, registry *ModelRegistry, provider core.Provider, providerName, providerType, modelID string, pricing *core.ModelPricing) {
	t.Helper()
	registry.RegisterProviderWithNameAndType(provider, providerName, providerType)
	info := &ModelInfo{
		Model: core.Model{
			ID:       modelID,
			Object:   "model",
			Metadata: &core.ModelMetadata{Pricing: pricing},
		},
		Provider:     provider,
		ProviderName: providerName,
		ProviderType: providerType,
	}
	if registry.modelsByProvider[providerName] == nil {
		registry.modelsByProvider[providerName] = make(map[string]*ModelInfo)
	}
	registry.modelsByProvider[providerName][modelID] = info
	if _, exists := registry.models[modelID]; !exists {
		registry.models[modelID] = info
	}
}

func TestRouter_IntelligentRouting_PicksCheaperProvider(t *testing.T) {
	registry := NewModelRegistry()
	cheap := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-east", chatResponse: &core.ChatResponse{ID: "east-resp", Model: "gpt-4o"}},
		p50:          100 * time.Millisecond,
	}
	pricey := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-west", chatResponse: &core.ChatResponse{ID: "west-resp", Model: "gpt-4o"}},
		p50:          100 * time.Millisecond,
	}
	registerModelWithMetadata(t, registry, cheap, "openai-east", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(1), OutputPerMtok: floatPtr(2)})
	registerModelWithMetadata(t, registry, pricey, "openai-west", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(10), OutputPerMtok: floatPtr(20)})

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	resp, err := r.ChatCompletion(context.Background(), &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "east-resp" {
		t.Fatalf("expected cheaper east provider, got %s", resp.ID)
	}
}

func TestRouter_IntelligentRouting_PicksFasterProvider(t *testing.T) {
	registry := NewModelRegistry()
	slow := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-slow", chatResponse: &core.ChatResponse{ID: "slow-resp", Model: "gpt-4o"}},
		p50:          500 * time.Millisecond,
	}
	fast := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-fast", chatResponse: &core.ChatResponse{ID: "fast-resp", Model: "gpt-4o"}},
		p50:          50 * time.Millisecond,
	}
	// Equal pricing so cost/latency scoring decides.
	pricing := &core.ModelPricing{InputPerMtok: floatPtr(5), OutputPerMtok: floatPtr(5)}
	registerModelWithMetadata(t, registry, slow, "openai-slow", "openai", "gpt-4o", pricing)
	registerModelWithMetadata(t, registry, fast, "openai-fast", "openai", "gpt-4o", pricing)

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	resp, err := r.ChatCompletion(context.Background(), &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "fast-resp" {
		t.Fatalf("expected faster provider, got %s", resp.ID)
	}
}

func TestRouter_IntelligentRouting_StrategyOverrideHonored(t *testing.T) {
	registry := NewModelRegistry()
	// Register pricey provider first, cheap provider second.
	pricey := &statsMockProvider{
		mockProvider: mockProvider{name: "pricey-first", chatResponse: &core.ChatResponse{ID: "pricey-resp", Model: "gpt-4o"}},
	}
	cheap := &statsMockProvider{
		mockProvider: mockProvider{name: "cheap-second", chatResponse: &core.ChatResponse{ID: "cheap-resp", Model: "gpt-4o"}},
	}
	registerModelWithMetadata(t, registry, pricey, "pricey-first", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(10), OutputPerMtok: floatPtr(10)})
	registerModelWithMetadata(t, registry, cheap, "cheap-second", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(1), OutputPerMtok: floatPtr(1)})

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Default balanced should pick cheap (cost dominates at 0.6/0.4).
	defaultResp, err := r.ChatCompletion(context.Background(), &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultResp.ID != "cheap-resp" {
		t.Fatalf("default balanced expected cheap, got %s", defaultResp.ID)
	}

	// Override to first_fit picks the first registered (pricey), ignoring cost.
	ctx := router.WithStrategyOverride(context.Background(), "first_fit")
	firstFitResp, err := r.ChatCompletion(ctx, &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if firstFitResp.ID != "pricey-resp" {
		t.Fatalf("first_fit override expected pricey (first registered), got %s", firstFitResp.ID)
	}
}

func TestRouter_IntelligentRouting_InvalidOverrideFallsBack(t *testing.T) {
	registry := NewModelRegistry()
	cheap := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-east", chatResponse: &core.ChatResponse{ID: "east-resp", Model: "gpt-4o"}},
	}
	pricey := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-west", chatResponse: &core.ChatResponse{ID: "west-resp", Model: "gpt-4o"}},
	}
	registerModelWithMetadata(t, registry, cheap, "openai-east", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(1), OutputPerMtok: floatPtr(2)})
	registerModelWithMetadata(t, registry, pricey, "openai-west", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(10), OutputPerMtok: floatPtr(20)})

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	ctx := router.WithStrategyOverride(context.Background(), "not-a-real-strategy")
	resp, err := r.ChatCompletion(ctx, &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invalid override falls back to default balanced, which picks cheap.
	if resp.ID != "east-resp" {
		t.Fatalf("invalid override should fall back to default (cheap), got %s", resp.ID)
	}
}

func TestRouter_IntelligentRouting_ExplicitProviderHintBypasses(t *testing.T) {
	registry := NewModelRegistry()
	cheap := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-east", chatResponse: &core.ChatResponse{ID: "east-resp", Model: "gpt-4o"}},
	}
	pricey := &statsMockProvider{
		mockProvider: mockProvider{name: "openai-west", chatResponse: &core.ChatResponse{ID: "west-resp", Model: "gpt-4o"}},
	}
	registerModelWithMetadata(t, registry, cheap, "openai-east", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(1), OutputPerMtok: floatPtr(2)})
	registerModelWithMetadata(t, registry, pricey, "openai-west", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(10), OutputPerMtok: floatPtr(20)})

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Explicit provider hint must route to the named provider regardless of cost.
	resp, err := r.ChatCompletion(context.Background(), &core.ChatRequest{
		Model:    "gpt-4o",
		Provider: "openai-west",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "west-resp" {
		t.Fatalf("explicit provider hint should bypass routing, got %s", resp.ID)
	}
}

func TestRouter_IntelligentRouting_SingleCandidateUnaffected(t *testing.T) {
	registry := NewModelRegistry()
	solo := &statsMockProvider{
		mockProvider: mockProvider{name: "openai", chatResponse: &core.ChatResponse{ID: "solo-resp", Model: "gpt-4o"}},
	}
	registerModelWithMetadata(t, registry, solo, "openai", "openai", "gpt-4o", nil)

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	resp, err := r.ChatCompletion(context.Background(), &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "solo-resp" {
		t.Fatalf("single candidate should route normally, got %s", resp.ID)
	}
}

func TestRouter_IntelligentRouting_ModelSyntaxProviderNotOverridden(t *testing.T) {
	registry := NewModelRegistry()
	cheap := &statsMockProvider{
		mockProvider: mockProvider{name: "cheap", chatResponse: &core.ChatResponse{ID: "cheap", Model: "gpt-4o"}},
	}
	pricey := &statsMockProvider{
		mockProvider: mockProvider{name: "pricey", chatResponse: &core.ChatResponse{ID: "pricey", Model: "gpt-4o"}},
	}
	registerModelWithMetadata(t, registry, cheap, "cheap", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(1), OutputPerMtok: floatPtr(1)})
	registerModelWithMetadata(t, registry, pricey, "pricey", "openai", "gpt-4o",
		&core.ModelPricing{InputPerMtok: floatPtr(10), OutputPerMtok: floatPtr(10)})

	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Provider specified via model syntax (pricey/gpt-4o) should not be overridden.
	resp, err := r.ChatCompletion(context.Background(), &core.ChatRequest{
		Model: "pricey/gpt-4o",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "pricey" {
		t.Fatalf("model-syntax provider (pricey/gpt-4o) should be respected, got %s", resp.ID)
	}
}

func TestRouter_FirstFit_RespectsRegistrationOrder(t *testing.T) {
	registry := NewModelRegistry()
	first := &statsMockProvider{
		mockProvider: mockProvider{name: "reg-first", chatResponse: &core.ChatResponse{ID: "first", Model: "gpt-4o"}},
	}
	second := &statsMockProvider{
		mockProvider: mockProvider{name: "reg-second", chatResponse: &core.ChatResponse{ID: "second", Model: "gpt-4o"}},
	}
	// Register in order first, then second.
	registerModelWithMetadata(t, registry, first, "reg-first", "openai", "gpt-4o", nil)
	registerModelWithMetadata(t, registry, second, "reg-second", "openai", "gpt-4o", nil)

	// Override strategy so order is the only decision factor.
	ctx := router.WithStrategyOverride(context.Background(), "first_fit")
	r, err := NewRouter(registry, WithStrategyRegistry(router.NewStrategyRegistry()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	resp, err := r.ChatCompletion(ctx, &core.ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "first" {
		t.Fatalf("first_fit should route to reg-first (registration order), got %s", resp.ID)
	}
}
