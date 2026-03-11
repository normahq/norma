package planner

import (
	"testing"
)

func TestFormatPlannerRunError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   error
		want string
	}{
		{
			name: "empty message",
			in:   errString("   "),
			want: "Planner run failed due to an unexpected error.",
		},
		{
			name: "rate limited resource exhausted",
			in:   errString("RESOURCE_EXHAUSTED: backend limit"),
			want: "Planner model quota/rate limit exceeded.\n\nRESOURCE_EXHAUSTED: backend limit\n\nTry again later or switch planner model/provider in .norma/config.yaml.",
		},
		{
			name: "rate limited 429",
			in:   errString("Error 429 from provider"),
			want: "Planner model quota/rate limit exceeded.\n\nError 429 from provider\n\nTry again later or switch planner model/provider in .norma/config.yaml.",
		},
		{
			name: "generic error",
			in:   errString("boom"),
			want: "Planner run failed.\n\nboom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatPlannerRunError(tc.in)
			if got != tc.want {
				t.Fatalf("formatPlannerRunError() = %q, want %q", got, tc.want)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
