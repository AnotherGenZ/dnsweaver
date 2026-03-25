package pihole

import (
	"os"
	"testing"
)

func TestLoadConfigFromMap(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]string
		wantErr bool
	}{
		{
			name: "valid API mode",
			config: map[string]string{
				"mode":     "api",
				"url":      "http://pihole.local",
				"password": "secret",
			},
			wantErr: false,
		},
		{
			name: "valid file mode",
			config: map[string]string{
				"mode":           "file",
				"config_dir":     "/etc/pihole",
				"config_file":    "custom.list",
				"reload_command": "pihole restartdns",
			},
			wantErr: false,
		},
		{
			name: "API mode with zone and TTL",
			config: map[string]string{
				"mode":     "api",
				"url":      "http://pihole.local",
				"password": "secret",
				"zone":     "example.com",
				"ttl":      "600",
			},
			wantErr: false,
		},
		{
			name: "missing mode uses default API",
			config: map[string]string{
				"url":      "http://pihole.local",
				"password": "secret",
			},
			wantErr: false,
		},
		{
			name: "access_mode preferred over mode",
			config: map[string]string{
				"access_mode":    "file",
				"mode":           "api",
				"config_dir":     "/etc/pihole",
				"config_file":    "custom.list",
				"reload_command": "pihole restartdns",
			},
			wantErr: false,
		},
		{
			name: "access_mode alone works",
			config: map[string]string{
				"access_mode": "api",
				"url":         "http://pihole.local",
				"password":    "secret",
			},
			wantErr: false,
		},
		{
			name: "invalid TTL",
			config: map[string]string{
				"mode":     "api",
				"url":      "http://pihole.local",
				"password": "secret",
				"ttl":      "invalid",
			},
			wantErr: true,
		},
		{
			name: "API mode missing URL",
			config: map[string]string{
				"mode":     "api",
				"password": "secret",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadConfigFromMap("test", tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfigFromMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && cfg == nil {
				t.Error("LoadConfigFromMap() returned nil config without error")
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Clean up environment after test
	defer func() {
		os.Unsetenv("DNSWEAVER_TEST_MODE")
		os.Unsetenv("DNSWEAVER_TEST_ACCESS_MODE")
		os.Unsetenv("DNSWEAVER_TEST_URL")
		os.Unsetenv("DNSWEAVER_TEST_PASSWORD")
		os.Unsetenv("DNSWEAVER_TEST_TTL")
		os.Unsetenv("DNSWEAVER_TEST_ZONE")
	}()

	tests := []struct {
		name     string
		envVars  map[string]string
		wantErr  bool
		wantMode Mode
		wantTTL  int
	}{
		{
			name: "API mode from ACCESS_MODE env (preferred)",
			envVars: map[string]string{
				"DNSWEAVER_TEST_ACCESS_MODE": "api",
				"DNSWEAVER_TEST_URL":         "http://pihole.local",
				"DNSWEAVER_TEST_PASSWORD":    "secret",
			},
			wantErr:  false,
			wantMode: ModeAPI,
			wantTTL:  DefaultTTL,
		},
		{
			name: "API mode from deprecated MODE env (backward compat)",
			envVars: map[string]string{
				"DNSWEAVER_TEST_MODE":     "api",
				"DNSWEAVER_TEST_URL":      "http://pihole.local",
				"DNSWEAVER_TEST_PASSWORD": "secret",
			},
			wantErr:  false,
			wantMode: ModeAPI,
			wantTTL:  DefaultTTL,
		},
		{
			name: "ACCESS_MODE takes precedence over MODE",
			envVars: map[string]string{
				"DNSWEAVER_TEST_ACCESS_MODE": "file",
				"DNSWEAVER_TEST_MODE":        "api",
			},
			wantErr:  false,
			wantMode: ModeFile,
			wantTTL:  DefaultTTL,
		},
		{
			name: "custom TTL",
			envVars: map[string]string{
				"DNSWEAVER_TEST_ACCESS_MODE": "api",
				"DNSWEAVER_TEST_URL":         "http://pihole.local",
				"DNSWEAVER_TEST_PASSWORD":    "secret",
				"DNSWEAVER_TEST_TTL":         "600",
			},
			wantErr:  false,
			wantMode: ModeAPI,
			wantTTL:  600,
		},
		{
			name: "file mode from env",
			envVars: map[string]string{
				"DNSWEAVER_TEST_ACCESS_MODE": "file",
				// Uses defaults for config_dir, config_file, reload_command
			},
			wantErr:  false,
			wantMode: ModeFile,
			wantTTL:  DefaultTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all test env vars first
			os.Unsetenv("DNSWEAVER_TEST_MODE")
			os.Unsetenv("DNSWEAVER_TEST_ACCESS_MODE")
			os.Unsetenv("DNSWEAVER_TEST_URL")
			os.Unsetenv("DNSWEAVER_TEST_PASSWORD")
			os.Unsetenv("DNSWEAVER_TEST_TTL")
			os.Unsetenv("DNSWEAVER_TEST_ZONE")

			// Set env vars for this test
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg, err := LoadConfig("test")
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if cfg.Mode != tt.wantMode {
					t.Errorf("LoadConfig() Mode = %v, want %v", cfg.Mode, tt.wantMode)
				}
				if cfg.TTL != tt.wantTTL {
					t.Errorf("LoadConfig() TTL = %v, want %v", cfg.TTL, tt.wantTTL)
				}
			}
		})
	}
}

func TestEnvPrefix(t *testing.T) {
	tests := []struct {
		name         string
		instanceName string
		want         string
	}{
		{
			name:         "simple name",
			instanceName: "pihole",
			want:         "DNSWEAVER_PIHOLE_",
		},
		{
			name:         "name with hyphen",
			instanceName: "pihole-dns",
			want:         "DNSWEAVER_PIHOLE_DNS_",
		},
		{
			name:         "lowercase name",
			instanceName: "my-pihole",
			want:         "DNSWEAVER_MY_PIHOLE_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envPrefix(tt.instanceName)
			if got != tt.want {
				t.Errorf("envPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_ConfigFilePath(t *testing.T) {
	config := &Config{
		ConfigDir:  "/etc/pihole",
		ConfigFile: "custom.list",
	}

	got := config.ConfigFilePath()
	want := "/etc/pihole/custom.list"

	if got != want {
		t.Errorf("ConfigFilePath() = %v, want %v", got, want)
	}
}

func TestValidate_EmptyModeDefaultsToAPI(t *testing.T) {
	// When Mode is empty and API fields are provided, Validate should
	// default to API mode instead of returning an error (#98).
	cfg := &Config{
		Mode:     "", // empty — should default to ModeAPI
		URL:      "http://pihole.local",
		Password: "secret",
		TTL:      DefaultTTL,
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should not error with empty mode and valid API fields: %v", err)
	}

	if cfg.Mode != ModeAPI {
		t.Errorf("Validate() should set Mode to ModeAPI, got %q", cfg.Mode)
	}
}

func TestValidate_InvalidModeStillErrors(t *testing.T) {
	cfg := &Config{
		Mode:     "invalid",
		URL:      "http://pihole.local",
		Password: "secret",
		TTL:      DefaultTTL,
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for invalid mode")
	}
}

func TestLoadConfig_DefaultModeWhenNotSet(t *testing.T) {
	// When neither ACCESS_MODE nor MODE is set, should default to api (#98)
	defer func() {
		os.Unsetenv("DNSWEAVER_DEFTEST_URL")
		os.Unsetenv("DNSWEAVER_DEFTEST_PASSWORD")
	}()

	os.Setenv("DNSWEAVER_DEFTEST_URL", "http://pihole.local")
	os.Setenv("DNSWEAVER_DEFTEST_PASSWORD", "secret")

	cfg, err := LoadConfig("deftest")
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}
	if cfg.Mode != ModeAPI {
		t.Errorf("LoadConfig() Mode = %q, want %q", cfg.Mode, ModeAPI)
	}
}

func TestLoadConfigFromMap_DefaultModeWhenNotSet(t *testing.T) {
	// When no mode key is present, should default to api (#98)
	cfg, err := LoadConfigFromMap("test", map[string]string{
		"url":      "http://pihole.local",
		"password": "secret",
	})
	if err != nil {
		t.Fatalf("LoadConfigFromMap() unexpected error: %v", err)
	}
	if cfg.Mode != ModeAPI {
		t.Errorf("LoadConfigFromMap() Mode = %q, want %q", cfg.Mode, ModeAPI)
	}
}
