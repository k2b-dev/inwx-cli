package dns

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeZoneAndNames(t *testing.T) {
	t.Parallel()

	zone, err := NormalizeZone("Bücher.Example.")
	if err != nil {
		t.Fatal(err)
	}
	if zone != "xn--bcher-kva.example." {
		t.Fatalf("unexpected zone %q", zone)
	}

	name, fqdn, err := NormalizeInputName("@", zone)
	if err != nil {
		t.Fatal(err)
	}
	if name != "@" || fqdn != zone {
		t.Fatalf("unexpected apex %q %q", name, fqdn)
	}

	name, fqdn, err = NormalizeInputName("WWW", zone)
	if err != nil {
		t.Fatal(err)
	}
	if name != "www" || fqdn != "www."+zone {
		t.Fatalf("unexpected relative name %q %q", name, fqdn)
	}
}

func TestNormalizeNameRejectsOutsideZone(t *testing.T) {
	t.Parallel()

	_, _, err := NormalizeInputName("outside.example.net.", "example.com.")
	if err == nil {
		t.Fatal("expected outside-zone error")
	}
}

func TestNameserverInfoFixture(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "inwx", "testdata", "nameserver-info-records.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			Records []RawRecord `json:"record"`
		} `json:"resData"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatal(err)
	}

	records := make([]Record, 0, len(envelope.Data.Records))
	for _, raw := range envelope.Data.Records {
		record, err := FromAPI("example.test.", raw)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != 5 {
		t.Fatalf("got %d records, want 5", len(records))
	}
	if records[4].Type != "MX" || records[4].Priority == nil || *records[4].Priority != 10 {
		t.Fatalf("unexpected MX record %#v", records[4])
	}
}

func TestUnknownRecordTypePassesThroughReadOnly(t *testing.T) {
	t.Parallel()

	record, err := FromAPI("example.test.", RawRecord{
		ID:       "91",
		Name:     "_sip._tcp.example.test",
		Type:     "SRV",
		Content:  "5 5060 sip.example.test",
		TTL:      300,
		Priority: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Value != "5 5060 sip.example.test" {
		t.Fatalf("unknown record content changed: %q", record.Value)
	}
	if record.Priority == nil || *record.Priority != 10 {
		t.Fatalf("unknown record priority missing: %#v", record)
	}
}

func TestStringIDAcceptsStringAndInteger(t *testing.T) {
	t.Parallel()

	for _, input := range []string{`"123"`, `123`} {
		var id StringID
		if err := json.Unmarshal([]byte(input), &id); err != nil {
			t.Fatal(err)
		}
		if id != "123" {
			t.Fatalf("got %q from %s", id, input)
		}
	}
}
