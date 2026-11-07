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
