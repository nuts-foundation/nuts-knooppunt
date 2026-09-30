package main

import "testing"

func TestEventTraceConfigRequiresCompletePrivateReceiver(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       map[string]string
		wantError bool
	}{
		{"disabled", nil, false},
		{"listener only", map[string]string{"SANDBOX_OTLP_LISTEN_ADDR": ":4318"}, true},
		{"token only", map[string]string{"SANDBOX_OTLP_TOKEN": "test-receiver-token"}, true},
		{"invalid listener", map[string]string{"SANDBOX_OTLP_LISTEN_ADDR": "broken", "SANDBOX_OTLP_TOKEN": "test-receiver-token", "SANDBOX_PRS_URL": "http://mock-prs:8080"}, true},
		{"complete", map[string]string{"SANDBOX_OTLP_LISTEN_ADDR": ":4318", "SANDBOX_OTLP_TOKEN": "test-receiver-token", "SANDBOX_PRS_URL": "http://mock-prs:8080"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := NewConfigFromEnv(envOf(tc.env))
			if (err != nil) != tc.wantError {
				t.Fatalf("config error = %v, want error %v", err, tc.wantError)
			}
			if err == nil && (cfg.eventTraces != nil) != (tc.env != nil) {
				t.Fatalf("receiver enabled = %v", cfg.eventTraces != nil)
			}
		})
	}
}
