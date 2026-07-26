package mutation

import (
	"errors"
	"testing"

	"github.com/k2b-dev/inwx-cli/internal/dns"
)

func TestCreateConflictsAndToken(t *testing.T) {
	t.Parallel()
	current := record("1", "www", "A", "192.0.2.1")
	requested := record("", "www", "A", "192.0.2.2")
	plan, err := Create("ote", []dns.Record{current}, requested)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Expect == "" || plan.Before != nil || plan.After == nil {
		t.Fatalf("unexpected plan %#v", plan)
	}
	reordered, err := Create("ote", []dns.Record{current}, requested)
	if err != nil || reordered.Expect != plan.Expect {
		t.Fatal("expect token is not deterministic")
	}

	for _, conflicting := range []dns.Record{
		record("2", "www", "A", "192.0.2.2"),
		record("3", "www", "CNAME", "target.example."),
	} {
		_, err := Create("ote", []dns.Record{conflicting}, requested)
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("expected conflict for %#v, got %v", conflicting, err)
		}
	}
}

func TestExactUpdateDeleteAndNoop(t *testing.T) {
	t.Parallel()
	current := record("7", "www", "A", "192.0.2.1")
	plan, err := Update("ote", []dns.Record{current}, "7", current)
	if err != nil || !plan.Noop {
		t.Fatalf("unexpected noop plan %#v err=%v", plan, err)
	}
	if !ExpectMatches(plan.Expect, plan.Expect) || ExpectMatches(plan.Expect, "bad") {
		t.Fatal("constant-time token comparison returned an invalid result")
	}

	for _, records := range [][]dns.Record{{}, {current, current}} {
		_, err := Delete("ote", records, current.Zone, "7")
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("expected exact ID conflict, got %v", err)
		}
	}

	duplicate := record("8", "www", "A", "192.0.2.2")
	requested := duplicate
	requested.ID = current.ID
	if _, err := Update("ote", []dns.Record{current, duplicate}, current.ID, requested); err == nil {
		t.Fatal("update accepted a duplicate record")
	}
}

func TestProtectedAndVerification(t *testing.T) {
	t.Parallel()
	protected := record("9", "@", "NS", "ns.example.")
	if _, err := Delete("ote", []dns.Record{protected}, protected.Zone, "9"); err == nil {
		t.Fatal("expected protected record error")
	}

	requested := record("", "www", "A", "192.0.2.1")
	if verified, ok := VerifyCreate([]dns.Record{record("10", "www", "A", "192.0.2.1")}, "10", requested); !ok || verified.ID != "10" {
		t.Fatalf("create was not verified: %#v", verified)
	}
	if !VerifyDelete(nil, "10") {
		t.Fatal("delete absence was not verified")
	}
}

func record(id, name, recordType, value string) dns.Record {
	priority := (*uint16)(nil)
	if recordType == "MX" {
		value = "mail.example."
		valuePriority := uint16(10)
		priority = &valuePriority
	}
	fqdn := name + ".example.test."
	if name == "@" {
		fqdn = "example.test."
	}
	return dns.Record{
		ID:       id,
		Zone:     "example.test.",
		Name:     name,
		FQDN:     fqdn,
		Type:     recordType,
		Value:    value,
		TTL:      3600,
		Priority: priority,
	}
}
