package dns

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

const (
	MinTTL = 300
	MaxTTL = 2147483647
)

type StringID string

func (id *StringID) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return errors.New("record ID is null")
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		if text == "" {
			return errors.New("record ID is empty")
		}
		*id = StringID(text)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return errors.New("record ID must be a string or integer")
	}
	if _, err := strconv.ParseUint(number.String(), 10, 64); err != nil {
		return errors.New("record ID must be a non-negative integer")
	}
	*id = StringID(number.String())
	return nil
}

type RawRecord struct {
	ID       StringID `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Content  string   `json:"content"`
	TTL      int      `json:"ttl"`
	Priority uint64   `json:"prio"`
}

type Record struct {
	ID       string  `json:"id"`
	Zone     string  `json:"zone"`
	Name     string  `json:"name"`
	FQDN     string  `json:"fqdn"`
	Type     string  `json:"type"`
	Value    string  `json:"value"`
	TTL      int     `json:"ttl"`
	Priority *uint16 `json:"priority,omitempty"`
}

func NormalizeZone(input string) (string, error) {
	if input == "" || input != strings.TrimSpace(input) {
		return "", errors.New("zone must be a non-empty name without surrounding whitespace")
	}
	if strings.HasSuffix(input, "..") {
		return "", errors.New("zone must contain at most one trailing dot")
	}

	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(input, "."))
	if err != nil {
		return "", fmt.Errorf("invalid zone: %w", err)
	}
	ascii = strings.ToLower(ascii)
	if err := validateDomain(ascii, false); err != nil {
		return "", fmt.Errorf("invalid zone: %w", err)
	}
	return ascii + ".", nil
}

func NormalizeInputName(input, zone string) (string, string, error) {
	if input == "@" {
		return "@", zone, nil
	}
	if input == "" || input != strings.TrimSpace(input) {
		return "", "", errors.New("record name must be non-empty without surrounding whitespace")
	}
	if strings.HasSuffix(input, "..") {
		return "", "", errors.New("record name must contain at most one trailing dot")
	}

	absoluteInput := strings.HasSuffix(input, ".")
	ascii, err := asciiRecordName(strings.TrimSuffix(input, "."))
	if err != nil {
		return "", "", fmt.Errorf("invalid record name: %w", err)
	}
	ascii = strings.ToLower(ascii)
	if err := validateDomain(ascii, true); err != nil {
		return "", "", fmt.Errorf("invalid record name: %w", err)
	}

	zoneName := strings.TrimSuffix(zone, ".")
	if ascii == zoneName {
		return "@", zone, nil
	}
	if strings.HasSuffix(ascii, "."+zoneName) {
		relative := strings.TrimSuffix(ascii, "."+zoneName)
		return relative, ascii + ".", nil
	}
	if absoluteInput {
		return "", "", errors.New("record name is outside the selected zone")
	}

	fqdn := ascii + "." + zoneName
	if err := validateDomain(fqdn, true); err != nil {
		return "", "", fmt.Errorf("invalid record name: %w", err)
	}
	return ascii, fqdn + ".", nil
}

func FromAPI(zone string, raw RawRecord) (Record, error) {
	zoneName := strings.TrimSuffix(zone, ".")
	asciiName, err := asciiRecordName(strings.TrimSuffix(raw.Name, "."))
	if err != nil {
		return Record{}, fmt.Errorf("invalid API record name: %w", err)
	}
	asciiName = strings.ToLower(asciiName)

	var relative string
	switch {
	case asciiName == zoneName:
		relative = "@"
	case strings.HasSuffix(asciiName, "."+zoneName):
		relative = strings.TrimSuffix(asciiName, "."+zoneName)
	default:
		return Record{}, errors.New("API record name is outside the selected zone")
	}

	recordType := strings.ToUpper(raw.Type)
	if recordType == "" || recordType != raw.Type {
		return Record{}, errors.New("API record type must be non-empty uppercase text")
	}
	if raw.TTL < MinTTL || raw.TTL > MaxTTL {
		return Record{}, fmt.Errorf("API record TTL %d is outside %d..%d", raw.TTL, MinTTL, MaxTTL)
	}

	value, err := normalizeAPIValue(recordType, raw.Content)
	if err != nil {
		return Record{}, fmt.Errorf("invalid %s record value: %w", recordType, err)
	}

	record := Record{
		ID:    string(raw.ID),
		Zone:  zone,
		Name:  relative,
		FQDN:  asciiName + ".",
		Type:  recordType,
		Value: value,
		TTL:   raw.TTL,
	}
	if record.ID == "" {
		return Record{}, errors.New("API record ID is empty")
	}
	if raw.Priority > 65535 {
		return Record{}, fmt.Errorf("API record priority %d exceeds 65535", raw.Priority)
	}
	if recordType == "MX" || raw.Priority != 0 {
		priority := uint16(raw.Priority)
		record.Priority = &priority
	}
	return record, nil
}

func NewRecord(
	zone string,
	name string,
	recordType string,
	value string,
	ttl int,
	priority *uint16,
) (Record, error) {
	canonicalZone, err := NormalizeZone(zone)
	if err != nil {
		return Record{}, err
	}
	canonicalName, fqdn, err := NormalizeInputName(name, canonicalZone)
	if err != nil {
		return Record{}, err
	}
	recordType = strings.ToUpper(recordType)
	if !IsMutableType(recordType) {
		return Record{}, errors.New("record type must be A, AAAA, CNAME, TXT, or MX")
	}
	if ttl < MinTTL || ttl > MaxTTL {
		return Record{}, fmt.Errorf("TTL must be between %d and %d", MinTTL, MaxTTL)
	}
	if recordType == "MX" && priority == nil {
		return Record{}, errors.New("priority is required for MX records")
	}
	if recordType != "MX" && priority != nil {
		return Record{}, errors.New("priority is only valid for MX records")
	}
	canonicalValue, err := normalizeValue(recordType, value)
	if err != nil {
		return Record{}, fmt.Errorf("invalid %s record value: %w", recordType, err)
	}
	return Record{
		Zone:     canonicalZone,
		Name:     canonicalName,
		FQDN:     fqdn,
		Type:     recordType,
		Value:    canonicalValue,
		TTL:      ttl,
		Priority: priority,
	}, nil
}

func IsMutableType(recordType string) bool {
	switch recordType {
	case "A", "AAAA", "CNAME", "TXT", "MX":
		return true
	default:
		return false
	}
}

func SortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		leftPriority, rightPriority := uint16(0), uint16(0)
		if left.Priority != nil {
			leftPriority = *left.Priority
		}
		if right.Priority != nil {
			rightPriority = *right.Priority
		}
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if left.Value != right.Value {
			return left.Value < right.Value
		}
		return left.ID < right.ID
	})
}

func normalizeValue(recordType, value string) (string, error) {
	switch recordType {
	case "A":
		address, err := netip.ParseAddr(value)
		if err != nil || !address.Is4() {
			return "", errors.New("expected an IPv4 address")
		}
		return address.String(), nil
	case "AAAA":
		address, err := netip.ParseAddr(value)
		if err != nil || !address.Is6() {
			return "", errors.New("expected an IPv6 address")
		}
		return address.String(), nil
	case "CNAME", "MX":
		if value == "" || value != strings.TrimSpace(value) {
			return "", errors.New("target must be non-empty without surrounding whitespace")
		}
		ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(value, "."))
		if err != nil {
			return "", err
		}
		ascii = strings.ToLower(ascii)
		if err := validateDomain(ascii, false); err != nil {
			return "", err
		}
		return ascii + ".", nil
	case "TXT":
		if strings.ContainsAny(value, "\x00\r\n") {
			return "", errors.New("TXT content cannot contain NUL or newlines")
		}
		return value, nil
	default:
		if strings.ContainsRune(value, '\x00') {
			return "", errors.New("record content cannot contain NUL")
		}
		return value, nil
	}
}

func normalizeAPIValue(recordType, value string) (string, error) {
	canonical, err := normalizeValue(recordType, value)
	if err == nil {
		return canonical, nil
	}
	if recordType != "CNAME" && recordType != "MX" {
		return "", err
	}
	if value == "" || value != strings.TrimSpace(value) {
		return "", err
	}
	target, fallbackErr := asciiRecordName(strings.TrimSuffix(value, "."))
	if fallbackErr != nil {
		return "", err
	}
	target = strings.ToLower(target)
	if fallbackErr := validateDomain(target, true); fallbackErr != nil {
		return "", err
	}
	return target + ".", nil
}

func asciiRecordName(name string) (string, error) {
	labels := strings.Split(name, ".")
	for index, label := range labels {
		if !strings.ContainsRune(label, '_') {
			ascii, err := idna.Lookup.ToASCII(label)
			if err != nil {
				return "", err
			}
			labels[index] = ascii
			continue
		}
		for _, character := range label {
			if character > 127 ||
				!(character == '_' ||
					character == '-' ||
					character >= 'a' && character <= 'z' ||
					character >= 'A' && character <= 'Z' ||
					character >= '0' && character <= '9') {
				return "", errors.New("underscore labels must contain ASCII letters, digits, hyphens, or underscores")
			}
		}
	}
	return strings.Join(labels, "."), nil
}

func validateDomain(name string, allowUnderscore bool) error {
	if name == "" || len(name) > 253 {
		return errors.New("domain length must be 1..253 bytes")
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return errors.New("domain labels must be 1..63 bytes")
		}
		if label == "*" {
			return errors.New("wildcard names are not supported")
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return errors.New("domain labels cannot start or end with a hyphen")
		}
		for _, character := range label {
			if character >= 'a' && character <= 'z' ||
				character >= '0' && character <= '9' ||
				character == '-' ||
				allowUnderscore && character == '_' {
				continue
			}
			return errors.New("domain contains an invalid character")
		}
	}
	return nil
}
