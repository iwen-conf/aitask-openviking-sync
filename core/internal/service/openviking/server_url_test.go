package openviking

import "testing"

func TestServerURLCandidates(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "root adds ov-api fallback",
			input: "https://openviking.example.com/",
			want:  []string{"https://openviking.example.com", "https://openviking.example.com/ov-api"},
		},
		{
			name:  "ov-api adds root fallback",
			input: "https://openviking.example.com/ov-api/",
			want:  []string{"https://openviking.example.com/ov-api", "https://openviking.example.com"},
		},
		{
			name:  "console rewrites to ov-api",
			input: "https://openviking.example.com/console/",
			want:  []string{"https://openviking.example.com/ov-api", "https://openviking.example.com"},
		},
		{
			name:  "console nested rewrites to ov-api",
			input: "https://openviking.example.com/console/api/v1",
			want:  []string{"https://openviking.example.com/ov-api", "https://openviking.example.com"},
		},
		{
			name:  "api-v1 suffix is trimmed",
			input: "https://openviking.example.com/ov-api/api/v1",
			want:  []string{"https://openviking.example.com/ov-api", "https://openviking.example.com"},
		},
		{
			name:  "custom prefix kept",
			input: "https://openviking.example.com/gateway",
			want:  []string{"https://openviking.example.com/gateway"},
		},
		{
			name:  "query and fragment removed",
			input: "https://openviking.example.com/ov-api/?a=1#x",
			want:  []string{"https://openviking.example.com/ov-api", "https://openviking.example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ServerURLCandidates(tc.input)
			if err != nil {
				t.Fatalf("ServerURLCandidates() error = %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len(candidates) = %d, want %d, got=%v", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("candidate[%d] = %q, want %q (all=%v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestNormalizeServerURL(t *testing.T) {
	got, err := NormalizeServerURL("https://openviking.example.com/console/")
	if err != nil {
		t.Fatalf("NormalizeServerURL() error = %v", err)
	}
	if got != "https://openviking.example.com/ov-api" {
		t.Fatalf("NormalizeServerURL() = %q, want %q", got, "https://openviking.example.com/ov-api")
	}
}

func TestServerURLCandidatesRejectsInvalidURL(t *testing.T) {
	if _, err := ServerURLCandidates("openviking.example.com"); err == nil {
		t.Fatal("ServerURLCandidates() error = nil, want error")
	}
	if _, err := ServerURLCandidates("   "); err == nil {
		t.Fatal("ServerURLCandidates(empty) error = nil, want error")
	}
}
