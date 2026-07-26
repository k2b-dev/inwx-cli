package inwx

import (
	"testing"
	"time"
)

func TestGenerateTOTPUsesRFC6238Vector(t *testing.T) {
	t.Parallel()

	code, err := generateTOTP(
		"GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		time.Unix(59, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if code != "287082" {
		t.Fatalf("got %s, want 287082", code)
	}
}

func TestGenerateTOTPRejectsMissingSecret(t *testing.T) {
	t.Parallel()

	if _, err := generateTOTP("", time.Unix(59, 0)); err == nil {
		t.Fatal("expected missing shared secret error")
	}
}
