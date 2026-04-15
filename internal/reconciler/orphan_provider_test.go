package reconciler

import (
	"context"
	"fmt"
	"testing"

	"gitlab.bluewillows.net/root/dnsweaver/pkg/provider"
	"gitlab.bluewillows.net/root/dnsweaver/pkg/source"
)

func TestAppendUnique(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		value string
		want  int // expected length
	}{
		{
			name:  "append to nil",
			slice: nil,
			value: "a",
			want:  1,
		},
		{
			name:  "append new value",
			slice: []string{"a"},
			value: "b",
			want:  2,
		},
		{
			name:  "skip duplicate",
			slice: []string{"a", "b"},
			value: "a",
			want:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendUnique(tt.slice, tt.value)
			if len(got) != tt.want {
				t.Errorf("appendUnique() len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestGetOrphanProviders_UsesStoredMapping(t *testing.T) {
	// When hostnameProviders has a mapping for the hostname, getOrphanProviders
	// should return those providers instead of using domain matching.
	logger := quietLogger()

	mockInternalDNS := newTestMockProvider("internal-dns")
	mockCloudflare := newTestMockProvider("cloudflare")

	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		switch cfg.Name {
		case "internal-dns":
			return mockInternalDNS, nil
		case "cloudflare":
			return mockCloudflare, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})

	// Register providers with patterns:
	// internal-dns matches *.local.example.com
	// cloudflare matches *.example.com
	err := reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.local.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		TTL:        300,
	})
	if err != nil {
		t.Fatalf("failed to create internal-dns instance: %v", err)
	}

	err = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "cloudflare",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "203.0.113.1",
		TTL:        300,
	})
	if err != nil {
		t.Fatalf("failed to create cloudflare instance: %v", err)
	}

	rec := &Reconciler{
		providers:      reg,
		logger:         logger,
		knownHostnames: make(map[string]struct{}),
		// Simulate previous reconciliation that routed app.local.example.com to internal-dns
		hostnameProviders: map[string][]string{
			"app.local.example.com": {"internal-dns"},
		},
	}

	// getOrphanProviders should use the stored mapping
	providers := rec.getOrphanProviders("app.local.example.com")
	if len(providers) != 1 {
		t.Fatalf("getOrphanProviders() returned %d providers, want 1", len(providers))
	}
	if providers[0].Name() != "internal-dns" {
		t.Errorf("getOrphanProviders() returned %q, want %q", providers[0].Name(), "internal-dns")
	}
}

func TestGetOrphanProviders_FallsBackToMatching(t *testing.T) {
	// When hostnameProviders has no mapping, getOrphanProviders should fall
	// back to domain-based matching.
	logger := quietLogger()

	mockProvider := newTestMockProvider("internal-dns")
	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(_ provider.FactoryConfig) (provider.Provider, error) {
		return mockProvider, nil
	})

	err := reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.local.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		TTL:        300,
	})
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}

	rec := &Reconciler{
		providers:         reg,
		logger:            logger,
		knownHostnames:    make(map[string]struct{}),
		hostnameProviders: nil, // no mapping — simulates first run after recovery
	}

	providers := rec.getOrphanProviders("app.local.example.com")
	if len(providers) != 1 {
		t.Fatalf("getOrphanProviders() returned %d providers, want 1", len(providers))
	}
	if providers[0].Name() != "internal-dns" {
		t.Errorf("getOrphanProviders() returned %q, want %q", providers[0].Name(), "internal-dns")
	}
}

func TestGetOrphanProviders_RemovedProvider(t *testing.T) {
	// When the stored mapping references a provider that no longer exists,
	// that provider should be silently skipped.
	logger := quietLogger()

	reg := provider.NewRegistry(logger)

	rec := &Reconciler{
		providers:      reg,
		logger:         logger,
		knownHostnames: make(map[string]struct{}),
		hostnameProviders: map[string][]string{
			"app.old.example.com": {"removed-provider"},
		},
	}

	providers := rec.getOrphanProviders("app.old.example.com")
	if len(providers) != 0 {
		t.Errorf("getOrphanProviders() returned %d providers, want 0 (removed provider)", len(providers))
	}
}

func TestCleanupOrphans_UsesProviderMapping(t *testing.T) {
	// Full integration test: when a hostname moves between providers,
	// the old provider's record should be cleaned up using the stored mapping.
	logger := quietLogger()

	mockInternalDNS := newTestMockProvider("internal-dns")
	mockCloudflare := newTestMockProvider("cloudflare")

	// Add the orphan record to internal-dns
	mockInternalDNS.AddRecord(provider.Record{
		Hostname: "app.local.example.com",
		Type:     provider.RecordTypeA,
		Target:   "10.0.0.1",
	})

	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		switch cfg.Name {
		case "internal-dns":
			return mockInternalDNS, nil
		case "cloudflare":
			return mockCloudflare, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})

	// internal-dns matches *.local.example.com
	err := reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.local.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       "authoritative", // Skip ownership check for simplicity
		TTL:        300,
	})
	if err != nil {
		t.Fatalf("create internal-dns: %v", err)
	}

	// cloudflare matches *.example.com
	err = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "cloudflare",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "203.0.113.1",
		TTL:        300,
	})
	if err != nil {
		t.Fatalf("create cloudflare: %v", err)
	}

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:    true,
			OwnershipTracking: false, // Don't require ownership for this test
		},
		// Previous state: app.local.example.com was routed to internal-dns
		knownHostnames: map[string]struct{}{
			"app.local.example.com": {},
		},
		hostnameProviders: map[string][]string{
			"app.local.example.com": {"internal-dns"},
		},
	}
	rec.syncAtomics()

	// Current state: hostname changed to app.example.com (different hostname)
	currentHostnames := map[string]*source.Hostname{
		"app.example.com": {Name: "app.example.com", Source: "traefik"},
	}

	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": {
				"app.local.example.com": {
					{Hostname: "app.local.example.com", Type: provider.RecordTypeA, Target: "10.0.0.1"},
				},
			},
		},
		logger: logger,
	}

	actions := rec.cleanupOrphans(context.Background(), currentHostnames, cache)

	// Should have attempted to delete from internal-dns using stored mapping
	var deleteActions []Action
	for _, a := range actions {
		if a.Type == ActionDelete {
			deleteActions = append(deleteActions, a)
		}
	}

	if len(deleteActions) == 0 {
		t.Fatal("cleanupOrphans() produced no delete actions for orphaned hostname")
	}

	foundInternalDNS := false
	for _, a := range deleteActions {
		if a.Provider == "internal-dns" && a.Hostname == "app.local.example.com" {
			foundInternalDNS = true
		}
	}
	if !foundInternalDNS {
		t.Errorf("cleanupOrphans() did not attempt to delete from internal-dns (stored provider mapping); actions: %v", deleteActions)
	}
}

func TestCleanupDetachedProviders_CleansUpPreviousProviderWhenNoLongerMatched(t *testing.T) {
	logger := quietLogger()

	mockInternalDNS := newTestMockProvider("internal-dns")
	mockInternalDNS.AddRecord(provider.Record{
		Hostname: "app.example.com",
		Type:     provider.RecordTypeA,
		Target:   "10.0.0.1",
	})

	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		if cfg.Name == "internal-dns" {
			return mockInternalDNS, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})

	err := reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})
	if err != nil {
		t.Fatalf("create internal-dns: %v", err)
	}

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:    true,
			OwnershipTracking: false,
		},
		knownHostnames: map[string]struct{}{
			"app.example.com": {},
		},
		hostnameProviders: map[string][]string{
			"app.example.com": {"internal-dns"},
		},
	}
	rec.syncAtomics()

	currentHostnames := map[string]*source.Hostname{
		"app.example.com": {Name: "app.example.com", Source: "traefik"},
	}
	currentProviderMapping := map[string][]string{}

	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": {
				"app.example.com": {
					{Hostname: "app.example.com", Type: provider.RecordTypeA, Target: "10.0.0.1"},
				},
			},
		},
		logger: logger,
	}

	actions := rec.cleanupDetachedProviders(context.Background(), currentHostnames, currentProviderMapping, map[string]detachedRoutingState{}, cache)

	var deleteActions []Action
	for _, action := range actions {
		if action.Type == ActionDelete && action.Status == StatusSuccess {
			deleteActions = append(deleteActions, action)
		}
	}

	if len(deleteActions) == 0 {
		t.Fatalf("cleanupDetachedProviders() produced no successful delete actions; actions: %+v", actions)
	}
	if deleteActions[0].Provider != "internal-dns" {
		t.Errorf("delete provider = %q, want internal-dns", deleteActions[0].Provider)
	}
	if deleteActions[0].Hostname != "app.example.com" {
		t.Errorf("delete hostname = %q, want app.example.com", deleteActions[0].Hostname)
	}
}

func TestCleanupDetachedProviders_DefersWhenCurrentProviderUnhealthy(t *testing.T) {
	logger := quietLogger()

	internal := newTestMockProvider("internal-dns")
	external := newTestMockProvider("external-dns")
	internal.AddRecord(provider.Record{Hostname: "app.example.com", Type: provider.RecordTypeA, Target: "10.0.0.1"})

	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		if cfg.Name == "internal-dns" {
			return internal, nil
		}
		if cfg.Name == "external-dns" {
			return external, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})

	_ = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})
	_ = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "external-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "203.0.113.10",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:    true,
			OwnershipTracking: false,
		},
		hostnameProviders: map[string][]string{
			"app.example.com": {"internal-dns", "external-dns"},
		},
	}
	rec.syncAtomics()

	currentHostnames := map[string]*source.Hostname{
		"app.example.com": {Name: "app.example.com", Source: "traefik"},
	}
	currentProviderMapping := map[string][]string{
		"app.example.com": {"external-dns"},
	}
	currentRoutingState := map[string]detachedRoutingState{
		"app.example.com": {
			hasCurrentProviderError: true,
		},
	}
	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": {
				"app.example.com": {
					{Hostname: "app.example.com", Type: provider.RecordTypeA, Target: "10.0.0.1"},
				},
			},
		},
		logger: logger,
	}

	actions := rec.cleanupDetachedProviders(context.Background(), currentHostnames, currentProviderMapping, currentRoutingState, cache)
	if len(actions) != 0 {
		t.Fatalf("expected deferred detached cleanup with no actions, got: %+v", actions)
	}

	if deleted := internal.GetDeleted(); len(deleted) != 0 {
		t.Fatalf("expected no internal deletions while current provider unhealthy, got: %+v", deleted)
	}
}

func TestCleanupDetachedProviders_CircuitBreakerByRatio(t *testing.T) {
	logger := quietLogger()
	internal := newTestMockProvider("internal-dns")
	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		if cfg.Name == "internal-dns" {
			return internal, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})

	_ = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:    true,
			OwnershipTracking: false,
		},
		hostnameProviders: make(map[string][]string),
	}
	rec.syncAtomics()

	currentHostnames := make(map[string]*source.Hostname)
	currentProviderMapping := make(map[string][]string)
	cacheRecords := make(map[string][]provider.Record)

	// 20 previous routed hostnames, 12 detached this cycle (60% > 50%, and >=10).
	for i := 0; i < 20; i++ {
		hostname := "app" + string(rune('a'+i)) + ".example.com"
		rec.hostnameProviders[hostname] = []string{"internal-dns"}
		if i < 12 {
			currentHostnames[hostname] = &source.Hostname{Name: hostname, Source: "traefik"}
			cacheRecords[hostname] = []provider.Record{
				{Hostname: hostname, Type: provider.RecordTypeA, Target: "10.0.0.1"},
			}
		}
	}

	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": cacheRecords,
		},
		logger: logger,
	}

	actions := rec.cleanupDetachedProviders(context.Background(), currentHostnames, currentProviderMapping, map[string]detachedRoutingState{}, cache)
	if len(actions) != 0 {
		t.Fatalf("expected detached cleanup breaker to skip actions, got: %+v", actions)
	}
	if deleted := internal.GetDeleted(); len(deleted) != 0 {
		t.Fatalf("expected no deletions when breaker triggers, got: %+v", deleted)
	}
}

func TestCleanupDetachedProviders_UsesCustomThresholds(t *testing.T) {
	logger := quietLogger()
	internal := newTestMockProvider("internal-dns")
	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		if cfg.Name == "internal-dns" {
			return internal, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})
	_ = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:                      true,
			OwnershipTracking:                   false,
			DetachedCleanupRatioThreshold:       0.8,
			DetachedCleanupRatioMinHostnames:    10,
			DetachedCleanupAbsoluteMaxHostnames: 1000,
		},
		hostnameProviders: make(map[string][]string),
	}
	rec.syncAtomics()

	currentHostnames := make(map[string]*source.Hostname)
	currentProviderMapping := make(map[string][]string)
	cacheRecords := make(map[string][]provider.Record)

	// 20 previous routed hostnames, 12 detached this cycle (60%).
	// Default threshold (50%) would trigger breaker; custom 80% should allow cleanup.
	for i := 0; i < 20; i++ {
		hostname := "app" + string(rune('a'+i)) + ".example.com"
		rec.hostnameProviders[hostname] = []string{"internal-dns"}
		if i < 12 {
			currentHostnames[hostname] = &source.Hostname{Name: hostname, Source: "traefik"}
			cacheRecords[hostname] = []provider.Record{
				{Hostname: hostname, Type: provider.RecordTypeA, Target: "10.0.0.1"},
			}
		}
	}

	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": cacheRecords,
		},
		logger: logger,
	}

	actions := rec.cleanupDetachedProviders(context.Background(), currentHostnames, currentProviderMapping, map[string]detachedRoutingState{}, cache)
	if len(actions) == 0 {
		t.Fatal("expected detached cleanup to proceed with custom thresholds")
	}
	if deleted := internal.GetDeleted(); len(deleted) == 0 {
		t.Fatal("expected deletions with custom thresholds")
	}
}

func TestCleanupDetachedProviders_CircuitBreakerByAbsoluteLimit(t *testing.T) {
	logger := quietLogger()
	internal := newTestMockProvider("internal-dns")
	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		if cfg.Name == "internal-dns" {
			return internal, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})
	_ = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:    true,
			OwnershipTracking: false,
		},
		hostnameProviders: make(map[string][]string),
	}
	rec.syncAtomics()

	currentHostnames := make(map[string]*source.Hostname)
	currentProviderMapping := make(map[string][]string)
	cacheRecords := make(map[string][]provider.Record)

	// 500 previous hostnames, 100 detached this cycle (20% ratio, but absolute cap hit).
	for i := 0; i < 500; i++ {
		hostname := fmt.Sprintf("svc-%d.example.com", i)
		rec.hostnameProviders[hostname] = []string{"internal-dns"}
		if i < 100 {
			currentHostnames[hostname] = &source.Hostname{Name: hostname, Source: "traefik"}
			cacheRecords[hostname] = []provider.Record{
				{Hostname: hostname, Type: provider.RecordTypeA, Target: "10.0.0.1"},
			}
		}
	}

	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": cacheRecords,
		},
		logger: logger,
	}

	actions := rec.cleanupDetachedProviders(context.Background(), currentHostnames, currentProviderMapping, map[string]detachedRoutingState{}, cache)
	if len(actions) != 0 {
		t.Fatalf("expected detached cleanup breaker to skip actions at absolute limit, got: %+v", actions)
	}
	if deleted := internal.GetDeleted(); len(deleted) != 0 {
		t.Fatalf("expected no deletions at absolute breaker limit, got: %+v", deleted)
	}
}

func TestCleanupDetachedProviders_AllowMassDeleteBypassesBreaker(t *testing.T) {
	logger := quietLogger()
	internal := newTestMockProvider("internal-dns")
	reg := provider.NewRegistry(logger)
	reg.RegisterFactory("mock", func(cfg provider.FactoryConfig) (provider.Provider, error) {
		if cfg.Name == "internal-dns" {
			return internal, nil
		}
		return newTestMockProvider(cfg.Name), nil
	})
	_ = reg.CreateInstance(provider.ProviderInstanceConfig{
		Name:       "internal-dns",
		TypeName:   "mock",
		Domains:    []string{"*.example.com"},
		RecordType: "A",
		Target:     "10.0.0.1",
		Mode:       provider.ModeAuthoritative,
		TTL:        300,
	})

	rec := &Reconciler{
		providers: reg,
		logger:    logger,
		config: Config{
			CleanupOrphans:                 true,
			OwnershipTracking:              false,
			DetachedCleanupAllowMassDelete: true,
		},
		hostnameProviders: make(map[string][]string),
	}
	rec.syncAtomics()

	currentHostnames := make(map[string]*source.Hostname)
	currentProviderMapping := make(map[string][]string)
	cacheRecords := make(map[string][]provider.Record)

	for i := 0; i < 12; i++ {
		hostname := fmt.Sprintf("mass-%d.example.com", i)
		rec.hostnameProviders[hostname] = []string{"internal-dns"}
		currentHostnames[hostname] = &source.Hostname{Name: hostname, Source: "traefik"}
		cacheRecords[hostname] = []provider.Record{
			{Hostname: hostname, Type: provider.RecordTypeA, Target: "10.0.0.1"},
		}
	}

	cache := &recordCache{
		records: map[string]map[string][]provider.Record{
			"internal-dns": cacheRecords,
		},
		logger: logger,
	}

	actions := rec.cleanupDetachedProviders(context.Background(), currentHostnames, currentProviderMapping, map[string]detachedRoutingState{}, cache)
	if len(actions) == 0 {
		t.Fatal("expected mass-delete override to allow detached cleanup actions")
	}
	if deleted := internal.GetDeleted(); len(deleted) == 0 {
		t.Fatal("expected deletions when mass-delete override is enabled")
	}
}
