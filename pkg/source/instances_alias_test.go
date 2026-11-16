package source

import (
	"reflect"
	"testing"

	"github.com/maxfield-allison/dnsweaver/pkg/workload"
)

// Fork feature: dnsweaver.provider / dnsweaver.dev/provider are aliases of the
// upstream instances selector, so 1.x-era labels keep working unchanged.
func TestWorkloadInstancesProviderAlias(t *testing.T) {
	for _, tt := range []struct {
		name                string
		labels, annotations map[string]string
		want                []string
		invalid             bool
	}{
		{name: "provider label", labels: map[string]string{"dnsweaver.provider": "internal"}, want: []string{"internal"}},
		{name: "provider list", labels: map[string]string{"dnsweaver.provider": "internal,external"}, want: []string{"external", "internal"}},
		{name: "provider annotation", annotations: map[string]string{"dnsweaver.dev/provider": "internal"}, want: []string{"internal"}},
		{name: "alias agrees with instances", labels: map[string]string{"dnsweaver.provider": "internal", "dnsweaver.instances": "internal"}, want: []string{"internal"}},
		{name: "alias contradicts instances", labels: map[string]string{"dnsweaver.provider": "internal", "dnsweaver.instances": "external"}, invalid: true},
		{name: "empty alias", labels: map[string]string{"dnsweaver.provider": ""}, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := workloadInstances(workload.Workload{Labels: tt.labels, Annotations: tt.annotations})
			if (err != nil) != tt.invalid || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selection=%v err=%v, want %v invalid=%v", got, err, tt.want, tt.invalid)
			}
		})
	}
}
