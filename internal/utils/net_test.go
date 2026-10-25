package utils

import "testing"

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"localhost:9981", true},
		{"127.0.0.1:9981", true},
		{"127.1.2.3:9981", true},
		{"[::1]:9981", true},
		{"8.8.8.8:9981", false},
		{"192.168.1.1:9981", false}, // private, but not loopback
		{"host.example.com:9981", false},
		{"127.0.0.1", false}, // missing port
		{"", false},
	}
	for _, tt := range tests {
		if got := IsLoopback(tt.addr); got != tt.want {
			t.Errorf("IsLoopback(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestIsLocal(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"localhost:9981", true},
		{"127.0.0.1:9981", true},
		{"10.0.0.5:9981", true},
		{"172.16.1.1:9981", true},
		{"172.32.1.1:9981", false}, // just past 172.16.0.0/12
		{"192.168.1.1:9981", true},
		{"100.64.0.1:9981", true},  // CGNAT
		{"169.254.1.1:9981", true}, // link-local
		{"[fd00::1]:9981", true},   // unique local IPv6
		{"8.8.8.8:9981", false},
		{"[2001:db8::1]:9981", false},
		{"host.example.com:9981", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsLocal(tt.addr); got != tt.want {
			t.Errorf("IsLocal(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestIsStdValid(t *testing.T) {
	valid := []string{
		"8.8.8.8:9981",
		"host.example.com:9981",
		"host.example.com.:9981", // trailing dot is allowed
		"[2001:db8::1]:9981",
		"127.0.0.1:9981", // loopback is allowed by IsStdValid
		"localhost:1",
		"a.b:65535",
	}
	for _, addr := range valid {
		if err := IsStdValid(addr); err != nil {
			t.Errorf("IsStdValid(%q) = %v, want nil", addr, err)
		}
	}

	invalid := []string{
		"",
		"8.8.8.8",                // no port
		"8.8.8.8:0",              // port out of range
		"8.8.8.8:65536",          // port out of range
		"8.8.8.8:port",           // port not an integer
		"0.0.0.0:9981",           // unspecified address
		"[::]:9981",              // unspecified address
		"example:9981",           // unqualified hostname
		"-host.example:9981",     // label starts with a hyphen
		"host-.example:9981",     // label ends with a hyphen
		"host..example:9981",     // empty label
		"host_name.example:9981", // invalid character
	}
	for _, addr := range invalid {
		if err := IsStdValid(addr); err == nil {
			t.Errorf("IsStdValid(%q) = nil, want an error", addr)
		}
	}
}

func TestIsValid(t *testing.T) {
	if err := IsValid("8.8.8.8:9981"); err != nil {
		t.Errorf("IsValid(8.8.8.8:9981) = %v, want nil", err)
	}
	// IsValid additionally forbids loopback addresses.
	for _, addr := range []string{"127.0.0.1:9981", "localhost:9981", "[::1]:9981"} {
		if err := IsValid(addr); err == nil {
			t.Errorf("IsValid(%q) = nil, want an error", addr)
		}
	}
}

func TestEqualIPNets(t *testing.T) {
	tests := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{}, nil, true},
		{[]string{"1.2.3.0/24"}, []string{"1.2.3.0/24"}, true},
		{[]string{"1.2.3.0/24", "5.6.7.0/24"}, []string{"5.6.7.0/24", "1.2.3.0/24"}, true}, // order-independent
		{[]string{"1.2.3.0/24"}, []string{"5.6.7.0/24"}, false},
		{[]string{"1.2.3.0/24"}, []string{"1.2.3.0/24", "5.6.7.0/24"}, false},
		{[]string{"1.2.3.0/24", "1.2.3.0/24"}, []string{"1.2.3.0/24", "5.6.7.0/24"}, false},
	}
	for _, tt := range tests {
		if got := EqualIPNets(tt.a, tt.b); got != tt.want {
			t.Errorf("EqualIPNets(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
