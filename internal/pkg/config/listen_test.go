package config

import "testing"

func TestValidateListen(t *testing.T) {
	tests := map[string]struct {
		listen  string
		wantErr bool
	}{
		"loopback ipv4":       {"127.0.0.1:8787", false},
		"loopback ipv6":       {"[::1]:8787", false},
		"localhost name":      {"localhost:8787", false},
		"all interfaces ipv4": {"0.0.0.0:8787", true},
		"empty host":          {":8787", true},
		"private ip":          {"192.168.1.10:8787", true},
		"missing port":        {"127.0.0.1", true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := Server{Listen: tt.listen}.ValidateListen()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateListen(%q) error = %v, wantErr %v", tt.listen, err, tt.wantErr)
			}
		})
	}
}
