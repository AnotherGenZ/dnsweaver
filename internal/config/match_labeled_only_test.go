package config

import (
	"os"
	"strings"
	"testing"
)

// Fork feature: DNSWEAVER_{NAME}_MATCH_LABELED_ONLY (opt-in provider instances).
func TestLoadInstanceConfig_MatchLabeledOnly(t *testing.T) {
	const instanceName = "optin-test"
	prefix := envPrefix(instanceName)
	for _, tt := range []struct {
		name, value, wantErrPart string
		want                     bool
	}{
		{name: "default off"},
		{name: "enabled", value: "true", want: true},
		{name: "disabled", value: "false"},
		{name: "invalid fails", value: "sometimes", wantErrPart: "MATCH_LABELED_ONLY"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearInstanceEnv(t, instanceName)
			defer clearInstanceEnv(t, instanceName)
			os.Setenv(prefix+"TYPE", "technitium")
			os.Setenv(prefix+"TARGET", "10.0.0.1")
			os.Setenv(prefix+"DOMAINS", "*.example.com")
			if tt.value != "" {
				os.Setenv(prefix+"MATCH_LABELED_ONLY", tt.value)
			}
			cfg, errs := loadInstanceConfig(instanceName, 300)
			if tt.wantErrPart != "" {
				if len(errs) == 0 || !strings.Contains(errs[0].Error(), tt.wantErrPart) {
					t.Fatalf("errors = %v, want one containing %q", errs, tt.wantErrPart)
				}
				return
			}
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if cfg.MatchLabeledOnly != tt.want {
				t.Fatalf("MatchLabeledOnly = %v, want %v", cfg.MatchLabeledOnly, tt.want)
			}
			if got := cfg.ToProviderConfig().MatchLabeledOnly; got != tt.want {
				t.Fatalf("provider config MatchLabeledOnly = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertFileProvider_MatchLabeledOnly(t *testing.T) {
	cfg, errs := convertFileProvider(FileProviderConfig{
		Name: "internal", Type: "technitium", RecordType: "A", Target: "10.0.0.1",
		Domains: []string{"*.example.com"}, MatchLabeledOnly: true,
	}, 300)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !cfg.MatchLabeledOnly {
		t.Fatal("YAML match_labeled_only was not carried into the instance config")
	}
}
