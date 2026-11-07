package database

import (
	"strings"
	"testing"
)

func TestRevisionApplicationName(t *testing.T) {
	const revision = "bible-api--0000123"
	if got := revisionApplicationName(revision); got != revision {
		t.Fatal(got)
	}
	a := revisionApplicationName(strings.Repeat("a", 64) + "one")
	b := revisionApplicationName(strings.Repeat("a", 64) + "two")
	if len(a) > 63 || len(b) > 63 || a == b {
		t.Fatalf("identity lost: %q %q", a, b)
	}
}

func TestRevisionApplicationNameRejectsNonPrintableOrNonASCII(t *testing.T) {
	for _, input := range []string{"revision\nprivate", "版本", "revision\x7f", "revision\x00"} {
		got := revisionApplicationName(input)
		if !strings.HasPrefix(got, "revision-") || len(got) != 25 || got != revisionApplicationName(input) {
			t.Fatalf("unsafe or unstable name %q", got)
		}
	}
}
