package money

import (
	"testing"
)

func TestParseAndFormat(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCents Cents
		wantErr   bool
		formatted string
	}{
		{"valid 94.00", "94.00", 9400, false, "94.00"},
		{"valid 0.50", "0.50", 50, false, "0.50"},
		{"valid integer 1", "1", 100, false, "1.00"},
		{"negative -1", "-1", 0, true, ""},
		{"three decimals 1.234", "1.234", 0, true, ""},
		{"scientific notation 1e2", "1e2", 0, true, ""},
		{"empty string", "", 0, true, ""},
		{"zero", "0.00", 0, false, "0.00"},
		{"single decimal 1.5", "1.5", 150, false, "1.50"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.wantCents {
				t.Errorf("Parse(%q) = %d, want %d", tc.input, got, tc.wantCents)
			}
			if got.Format() != tc.formatted {
				t.Errorf("Format() = %q, want %q", got.Format(), tc.formatted)
			}
		})
	}
}
