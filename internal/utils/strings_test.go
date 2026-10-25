package utils

import "testing"

func TestCapitalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"a", "A"},
		{"mainnet", "Mainnet"},
		{"Zen", "Zen"},
		{"two words", "Two words"},
	}
	for _, tt := range tests {
		if got := Capitalize(tt.in); got != tt.want {
			t.Errorf("Capitalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
