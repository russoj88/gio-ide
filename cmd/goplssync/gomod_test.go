package main

import "testing"

func TestVersionBelow(t *testing.T) {
	tests := []struct {
		current string
		floor   string
		want    bool
	}{
		{current: "", floor: "v0.39.0", want: true},
		{current: "v0.38.0", floor: "v0.39.0", want: true},
		{current: "v0.39.0", floor: "v0.39.0", want: false},
		{current: "v0.40.0", floor: "v0.39.0", want: false},
		{current: "v1.10.0", floor: "v1.10.1", want: true},
		{current: "v1.10.1", floor: "v1.10.1", want: false},
		{current: "not-a-version", floor: "v0.39.0", want: true},
	}
	for _, tt := range tests {
		if got := versionBelow(tt.current, tt.floor); got != tt.want {
			t.Errorf("versionBelow(%q, %q) = %v, want %v", tt.current, tt.floor, got, tt.want)
		}
	}
}
