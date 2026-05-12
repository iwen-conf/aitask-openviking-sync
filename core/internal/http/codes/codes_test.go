package codes

import "testing"

func TestStandard21ErrorCodes(t *testing.T) {
	if got, want := len(Standard21), 21; got != want {
		t.Fatalf("len(Standard21) = %d, want %d", got, want)
	}
	seen := map[string]struct{}{}
	for _, code := range Standard21 {
		if code == "" {
			t.Fatalf("code should not be empty")
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("duplicate code: %s", code)
		}
		seen[code] = struct{}{}
	}
}
