package alert

import "testing"

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com/hook", false},
		{"missing scheme", "example.com/hook", true},
		{"invalid scheme", "ftp://example.com/hook", true},
		{"loopback IP", "http://127.0.0.1/hook", true},
		{"loopback host", "http://localhost/hook", true},
		{"private IPv4", "http://10.0.0.5/hook", true},
		{"link-local", "http://169.254.169.254/hook", true},
		{"unspecified", "http://0.0.0.0/hook", true},
		{"loopback IPv6", "http://[::1]/hook", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWebhookURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error for %q, got %v", tc.url, err)
			}
		})
	}
}
