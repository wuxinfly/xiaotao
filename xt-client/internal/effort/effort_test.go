package effort

import "testing"

func TestParseAliases(t *testing.T) {
	cases := []struct {
		input string
		want  Level
	}{
		{"none", Low}, {"minimal", Low}, {"low", Low}, {"light", Low},
		{"medium", Medium}, {"high", High},
		{"ultra", XHigh}, {"xhigh", XHigh}, {"extra_high", XHigh}, {"max", Max},
		{" High ", High},
	}
	for _, tc := range cases {
		if got, ok := Parse(tc.input); !ok || got != tc.want {
			t.Fatalf("Parse(%q) = %q, %v; want %q", tc.input, got, ok, tc.want)
		}
	}
}

func TestParseEmptyAndUnknownFollowDefault(t *testing.T) {
	if _, ok := Parse(""); ok {
		t.Fatal("empty effort should follow provider default")
	}
	if _, ok := Parse("bogus"); ok {
		t.Fatal("unknown effort should follow provider default")
	}
}

func TestNearestPrefersClosestOfferedLevel(t *testing.T) {
	offered := []string{"light", "high", "extra_high"}
	if got := Nearest(Medium, offered); got != "high" {
		t.Fatalf("Nearest(medium) = %q, want high", got)
	}
	if got := Nearest(XHigh, offered); got != "extra_high" {
		t.Fatalf("Nearest(xhigh) = %q, want extra_high", got)
	}
}

func TestNearestFallsBackToFirstOffered(t *testing.T) {
	if got := Nearest(Low, []string{"medium", "max"}); got != "medium" {
		t.Fatalf("Nearest(low) = %q, want medium", got)
	}
	if got := Nearest(Low, []string{"not-a-level"}); got != "" {
		t.Fatalf("Nearest over unparsable levels = %q, want empty", got)
	}
}
