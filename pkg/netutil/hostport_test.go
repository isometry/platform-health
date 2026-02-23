package netutil

import "testing"

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "valid localhost",
			input:    "localhost:8080",
			wantHost: "localhost",
			wantPort: 8080,
			wantErr:  false,
		},
		{
			name:     "valid IP",
			input:    "192.168.1.1:443",
			wantHost: "192.168.1.1",
			wantPort: 443,
			wantErr:  false,
		},
		{
			name:     "valid hostname",
			input:    "example.com:9090",
			wantHost: "example.com",
			wantPort: 9090,
			wantErr:  false,
		},
		{
			name:     "empty host",
			input:    ":8080",
			wantHost: "",
			wantPort: 8080,
			wantErr:  false,
		},
		{
			name:    "missing port",
			input:   "localhost",
			wantErr: true,
		},
		{
			name:    "invalid port",
			input:   "localhost:abc",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := ParseHostPort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHostPort(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if host != tt.wantHost {
					t.Errorf("ParseHostPort(%q) host = %q, want %q", tt.input, host, tt.wantHost)
				}
				if port != tt.wantPort {
					t.Errorf("ParseHostPort(%q) port = %d, want %d", tt.input, port, tt.wantPort)
				}
			}
		})
	}
}
