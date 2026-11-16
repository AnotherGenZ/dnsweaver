package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/maxfield-allison/dnsweaver/pkg/provider"
	"github.com/maxfield-allison/dnsweaver/pkg/source"
	"github.com/maxfield-allison/dnsweaver/pkg/workload"
	native "github.com/maxfield-allison/dnsweaver/sources/dnsweaver"
)

func TestRegressionTXTConvergence(t *testing.T) {
	w := workload.Workload{ID: "regression", Name: "regression", Platform: workload.PlatformDocker, Labels: map[string]string{
		"dnsweaver.records.verify.hostname": "app.example.com",
		"dnsweaver.records.verify.type":     "TXT",
		"dnsweaver.records.verify.target":   "verification=regression",
	}}
	h, err := native.New().Extract(context.Background(), w)
	if err != nil || len(h) != 1 {
		t.Fatalf("real source: %v %v", h, err)
	}
	s := newTestMockSource("dnsweaver", h...)
	r, _, _ := liveSetHarness(t, setTestCapabilities(true, false), s, []workload.Workload{w})
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range second.Actions {
		if a.Type == ActionCreate || a.Type == ActionUpdate {
			t.Errorf("unchanged TXT caused write: %+v", a)
		}
	}
}

func TestRegressionCrossTypeFailureRestoresAnswer(t *testing.T) {
	s := newTestMockSource("native", *hintedA("app.example.com", "192.0.2.10"))
	r, m, _ := liveSetHarness(t, setTestCapabilities(true, false), s, []workload.Workload{{ID: "regression", Name: "regression", Platform: workload.PlatformDocker}})
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.hostnames = source.Hostnames{{Name: "app.example.com", Source: "native", RecordHints: &source.RecordHints{Type: "CNAME", Target: "new.example.com"}}}
	m.createFn = func(_ context.Context, record provider.Record) error {
		if record.Type == provider.RecordTypeCNAME {
			return errors.New("replacement rejected")
		}
		return nil
	}
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	records, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range records {
		if rec.Hostname == "app.example.com" && rec.Type == provider.RecordTypeA {
			return
		}
	}
	t.Fatalf("old answer absent after replacement failed: actions=%+v remaining=%+v", result.Actions, records)
}

func TestRegressionUnownedTXTPreserved(t *testing.T) {
	manual := provider.Record{Hostname: "app.example.com", Type: provider.RecordTypeTXT, Target: "v=spf1 -all", TTL: 300}
	claim := &source.Hostname{Name: manual.Hostname, Source: "native", RecordHints: &source.RecordHints{Type: "TXT", Target: "verification=regression"}}
	r, m, set, cache := setTestHarness(t, provider.ModeAuthoritative, setTestCapabilities(true, false), []provider.Record{manual}, claim)
	actions := r.reconcileDesiredSet(context.Background(), set, cache, nil)
	for _, rec := range m.GetDeleted() {
		if rec.Target == manual.Target {
			t.Fatalf("unowned TXT deleted: %+v", actions)
		}
	}
}

func TestRegressionMailRecordsShareOneHostname(t *testing.T) {
	labels := map[string]string{}
	declare := func(name, recordType, target, priority string) {
		labels["dnsweaver.records."+name+".hostname"] = "mail.example.com"
		labels["dnsweaver.records."+name+".type"] = recordType
		labels["dnsweaver.records."+name+".target"] = target
		if priority != "" {
			labels["dnsweaver.records."+name+".priority"] = priority
		}
	}
	declare("mx1", "MX", "mx1.example.net", "10")
	declare("mx2", "MX", "mx2.example.net", "20")
	declare("spf", "TXT", "v=spf1 include:example.net -all", "")
	declare("verify", "TXT", "verification=regression", "")
	w := workload.Workload{ID: "mail", Name: "mail", Platform: workload.PlatformDocker, Labels: labels}
	h, err := native.New().Extract(context.Background(), w)
	if err != nil || len(h) != 4 {
		t.Fatalf("real source: %v %v", h, err)
	}

	caps := setTestCapabilities(true, false)
	caps.SupportedRecordTypes = append(caps.SupportedRecordTypes, provider.RecordTypeMX)
	r, _, _ := liveSetHarness(t, caps, newTestMockSource("dnsweaver", h...), []workload.Workload{w})
	first, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := first.Created(); len(got) != 4 {
		t.Fatalf("created = %+v, want both MX and both TXT members", first.Actions)
	}
	second, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range second.Actions {
		if a.Type != ActionSkip {
			t.Errorf("unchanged mail records caused a write: %+v", a)
		}
	}
}
