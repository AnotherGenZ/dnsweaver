package reconciler

import (
	"context"
	"testing"

	"github.com/maxfield-allison/dnsweaver/pkg/provider"
	"github.com/maxfield-allison/dnsweaver/pkg/source"
	"github.com/maxfield-allison/dnsweaver/pkg/workload"
	native "github.com/maxfield-allison/dnsweaver/sources/dnsweaver"
	"github.com/maxfield-allison/dnsweaver/sources/traefik"
)

// Fork feature: MATCH_LABELED_ONLY. Mirrors the homelab split-horizon setup:
// "internal" (Technitium, opt-in) and "external" (Cloudflare) both match *.example.com.
func optInHarness(t *testing.T, labels map[string]string) (*Reconciler, *testMockWorkloadLister, map[string]*testMockProvider) {
	t.Helper()
	internal, external := newTestMockProvider("internal"), newTestMockProvider("external")
	for _, p := range []*testMockProvider{internal, external} {
		identity := provider.ProviderIdentity{Type: "mock", Endpoint: p.Name(), Zone: "example.com"}
		p.identity = &identity
	}
	providers := testProviderRegistry(quietLogger(), internal, external)
	providers.SetInstanceID("test-instance")
	for _, c := range []provider.ProviderInstanceConfig{
		{Name: "internal", TypeName: "mock", RecordType: provider.RecordTypeA, Target: "100.64.0.1", TTL: 300,
			Mode: provider.ModeManaged, Domains: []string{"*.example.com"}, MatchLabeledOnly: true},
		{Name: "external", TypeName: "mock", RecordType: provider.RecordTypeA, Target: "192.0.2.1", TTL: 300,
			Mode: provider.ModeManaged, Domains: []string{"*.example.com"}},
	} {
		if err := providers.CreateInstance(c); err != nil {
			t.Fatal(err)
		}
	}
	sources := source.NewRegistry(quietLogger())
	if err := sources.Register(traefik.New()); err != nil {
		t.Fatal(err)
	}
	if err := sources.Register(native.New()); err != nil {
		t.Fatal(err)
	}
	all := map[string]string{"traefik.http.routers.app.rule": "Host(`app.example.com`)"}
	for k, v := range labels {
		all[k] = v
	}
	lister := newTestMockWorkloadLister(workload.PlatformDocker)
	lister.workloads = []workload.Workload{{ID: "app", Name: "app", Platform: workload.PlatformDocker, Labels: all}}
	cfg := DefaultConfig()
	cfg.InstanceID = "test-instance"
	r := New([]workload.Lister{lister}, sources, providers, WithConfig(cfg), WithLogger(quietLogger()))
	return r, lister, map[string]*testMockProvider{"internal": internal, "external": external}
}

func createdProviders(res *Result) map[string]int {
	out := map[string]int{}
	for _, a := range res.Created() {
		out[a.Provider]++
	}
	return out
}

func TestMatchLabeledOnlyRouting(t *testing.T) {
	for _, tt := range []struct {
		name   string
		labels map[string]string
		want   map[string]int
	}{
		{name: "unlabeled skips the opt-in instance", want: map[string]int{"external": 1}},
		{name: "fork provider label selects it", labels: map[string]string{"dnsweaver.provider": "internal"}, want: map[string]int{"internal": 1}},
		{name: "upstream instances label selects it", labels: map[string]string{"dnsweaver.instances": "internal"}, want: map[string]int{"internal": 1}},
		{name: "both selected", labels: map[string]string{"dnsweaver.provider": "internal,external"}, want: map[string]int{"internal": 1, "external": 1}},
		{name: "named record targets it", labels: map[string]string{
			"dnsweaver.records.extra.hostname": "extra.example.com", "dnsweaver.records.extra.provider": "internal"},
			want: map[string]int{"internal": 1, "external": 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, _, _ := optInHarness(t, tt.labels)
			got := safetyReconcile(t, r)
			if c := createdProviders(got); len(c) != len(tt.want) || c["internal"] != tt.want["internal"] || c["external"] != tt.want["external"] {
				t.Fatalf("created per provider = %v, want %v (actions %v)", c, tt.want, got.Actions)
			}
		})
	}
}

// Dropping the label must retire the opt-in record (it is no longer selected) and
// publish the default route instead; re-adding it restores the opt-in record.
func TestMatchLabeledOnlyRouteChange(t *testing.T) {
	r, lister, backends := optInHarness(t, map[string]string{"dnsweaver.provider": "internal"})
	safetyReconcile(t, r)
	delete(lister.workloads[0].Labels, "dnsweaver.provider")
	got := safetyReconcile(t, r)
	if len(got.Deleted()) != 1 || got.Deleted()[0].Provider != "internal" {
		t.Fatalf("expected the internal record to be retired: %v", got.Actions)
	}
	if c := createdProviders(got); c["external"] != 1 {
		t.Fatalf("expected the default (external) route: %v", got.Actions)
	}
	if recs, _ := backends["internal"].List(context.Background()); len(recs) != 0 {
		t.Fatalf("internal still has records: %v", recs)
	}
	lister.workloads[0].Labels["dnsweaver.provider"] = "internal"
	got = safetyReconcile(t, r)
	if c := createdProviders(got); c["internal"] != 1 {
		t.Fatalf("re-selecting internal should recreate it: %v", got.Actions)
	}
}

// The single-hostname API path has no workload labels: opt-in instances never match.
func TestMatchLabeledOnlySingleHostnamePath(t *testing.T) {
	r, _, _ := optInHarness(t, nil)
	actions := r.ensureRecord(context.Background(), &source.Hostname{Name: "api.example.com", Source: "api"}, nil)
	for _, a := range actions {
		if a.Provider == "internal" {
			t.Fatalf("opt-in instance received an unselected hostname: %v", actions)
		}
	}
	selected := r.ensureRecord(context.Background(), &source.Hostname{Name: "sel.example.com", Source: "api", Instances: []string{"internal"}}, nil)
	if len(selected) == 0 || selected[0].Provider != "internal" {
		t.Fatalf("explicitly selected hostname should reach the opt-in instance: %v", selected)
	}
}
