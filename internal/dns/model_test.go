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

func TestFromAPIPreservesNonCanonicalTargetReadOnly(t *testing.T) {
	t.Parallel()

	record, err := FromAPI("kolb-antik.com.", RawRecord{
		ID:      "92",
		Name:    "gw.it.kolb-antik.com",
		Type:    "CNAME",
		Content: "109_70_197_43.rz.it.kolb-antik.com",
		TTL:     300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "92" || record.Value != "109_70_197_43.rz.it.kolb-antik.com." {
		t.Fatalf("provider target was not preserved: %#v", record)
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

func TestNewRecordNormalizesMutableTypes(t *testing.T) {
	t.Parallel()
	priority := uint16(10)
	cases := []struct {
		recordType string
		value      string
		priority   *uint16
		want       string
	}{
		{"A", "192.0.2.1", nil, "192.0.2.1"},
		{"AAAA", "2001:0db8::1", nil, "2001:db8::1"},
		{"CNAME", "Target.Example", nil, "target.example."},
		{"TXT", "exact text", nil, "exact text"},
		{"MX", "Mail.Example.", &priority, "mail.example."},
	}
	for _, test := range cases {
		test := test
		t.Run(test.recordType, func(t *testing.T) {
			record, err := NewRecord(
				"example.test", "WWW", test.recordType, test.value, 300, test.priority,
			)
			if err != nil {
				t.Fatal(err)
			}
			if record.Name != "www" || record.Value != test.want {
				t.Fatalf("unexpected record %#v", record)
			}
		})
	}
}

func TestNewRecordRejectsMalformedAndUnsupportedInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		recordType string
		value      string
		ttl        int
		priority   *uint16
	}{
		{"A", "2001:db8::1", 300, nil},
		{"AAAA", "192.0.2.1", 300, nil},
		{"CNAME", "109_70_197_43.rz.example.test", 300, nil},
		{"TXT", "line\nbreak", 300, nil},
		{"SRV", "value", 300, nil},
		{"A", "192.0.2.1", 299, nil},
		{"MX", "mail.example", 300, nil},
	}
	for _, test := range cases {
		if _, err := NewRecord(
			"example.test", "www", test.recordType, test.value, test.ttl, test.priority,
		); err == nil {
			t.Fatalf("accepted invalid %s value %q", test.recordType, test.value)
		}
	}
}
