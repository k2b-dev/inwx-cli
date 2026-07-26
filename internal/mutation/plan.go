package mutation

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/k2b-dev/inwx-cli/internal/dns"
)

const schemaVersion = "inwx.cli/v1"

type Plan struct {
	Operation string      `json:"operation"`
	Zone      string      `json:"zone"`
	Before    *dns.Record `json:"before"`
	After     *dns.Record `json:"after"`
	Expect    string      `json:"expect"`
	Noop      bool        `json:"noop"`
}

type ConflictError struct {
	Message string
}

func (err *ConflictError) Error() string { return err.Message }

func Create(environment string, records []dns.Record, requested dns.Record) (Plan, error) {
	if requested.ID != "" {
		return Plan{}, errors.New("new record must not have an ID")
	}
	relevant := ownerRecords(records, requested.Name, "")
	for _, current := range relevant {
		if sameRecordState(current, requested, false) {
			return Plan{}, &ConflictError{Message: "an identical record already exists"}
		}
		if current.Type == "CNAME" || requested.Type == "CNAME" {
			return Plan{}, &ConflictError{Message: "CNAME records cannot coexist with another record at the same name"}
		}
	}
	plan := Plan{Operation: "create", Zone: requested.Zone, After: recordPointer(requested)}
	plan.Expect = token(environment, plan, relevant)
	return plan, nil
}

func Update(
	environment string,
	records []dns.Record,
	id string,
	requested dns.Record,
) (Plan, error) {
	current, err := exactRecord(records, id)
	if err != nil {
		return Plan{}, err
	}
	if !dns.IsMutableType(current.Type) {
		return Plan{}, &ConflictError{Message: fmt.Sprintf("record type %s is not mutable in v0.1", current.Type)}
	}
	if current.Type != requested.Type || current.ID != requested.ID || current.Zone != requested.Zone {
		return Plan{}, errors.New("updated record identity does not match current record")
	}
	for _, other := range ownerRecords(records, requested.Name, current.ID) {
		if sameRecordState(other, requested, false) {
			return Plan{}, &ConflictError{Message: "the update would duplicate an existing record"}
		}
		if other.Type == "CNAME" || requested.Type == "CNAME" {
			return Plan{}, &ConflictError{Message: "CNAME records cannot coexist with another record at the same name"}
		}
	}
	plan := Plan{
		Operation: "update",
		Zone:      current.Zone,
		Before:    recordPointer(current),
		After:     recordPointer(requested),
		Noop:      sameRecordState(current, requested, true),
	}
	plan.Expect = token(environment, plan, []dns.Record{current})
	return plan, nil
}

func Delete(environment string, records []dns.Record, zone string, id string) (Plan, error) {
	current, err := exactRecord(records, id)
	if err != nil {
		return Plan{}, err
	}
	if current.Zone != zone {
		return Plan{}, &ConflictError{Message: "record ID is outside the selected zone"}
	}
	if !dns.IsMutableType(current.Type) {
		return Plan{}, &ConflictError{Message: fmt.Sprintf("record type %s is protected in v0.1", current.Type)}
	}
	plan := Plan{
		Operation: "delete",
		Zone:      zone,
		Before:    recordPointer(current),
	}
	plan.Expect = token(environment, plan, []dns.Record{current})
	return plan, nil
}

func ExpectMatches(expected, actual string) bool {
	if len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func VerifyCreate(records []dns.Record, id string, requested dns.Record) (*dns.Record, bool) {
	current, err := exactRecord(records, id)
	if err != nil || !sameRecordState(current, requested, false) {
		return nil, false
	}
	return recordPointer(current), true
}

func RecoverCreate(records []dns.Record, requested dns.Record) (*dns.Record, bool) {
	matches := make([]dns.Record, 0, 1)
	for _, current := range records {
		if sameRecordState(current, requested, false) {
			matches = append(matches, current)
		}
	}
	if len(matches) != 1 {
		return nil, false
	}
	return recordPointer(matches[0]), true
}

func VerifyUpdate(records []dns.Record, requested dns.Record) (*dns.Record, bool) {
	current, err := exactRecord(records, requested.ID)
	if err != nil || !sameRecordState(current, requested, true) {
		return nil, false
	}
	return recordPointer(current), true
}

func VerifyDelete(records []dns.Record, id string) bool {
	_, err := exactRecord(records, id)
	var conflict *ConflictError
	return errors.As(err, &conflict) && conflict.Message == "record ID does not exist"
}

func ownerRecords(records []dns.Record, name, excludeID string) []dns.Record {
	relevant := make([]dns.Record, 0)
	for _, record := range records {
		if record.Name == name && record.ID != excludeID {
			relevant = append(relevant, record)
		}
	}
	dns.SortRecords(relevant)
	return relevant
}

func exactRecord(records []dns.Record, id string) (dns.Record, error) {
	if id == "" {
		return dns.Record{}, errors.New("record ID is required")
	}
	matches := make([]dns.Record, 0, 1)
	for _, record := range records {
		if record.ID == id {
			matches = append(matches, record)
		}
	}
	switch len(matches) {
	case 0:
		return dns.Record{}, &ConflictError{Message: "record ID does not exist"}
	case 1:
		return matches[0], nil
	default:
		return dns.Record{}, &ConflictError{Message: "record ID is ambiguous"}
	}
}

func sameRecordState(left, right dns.Record, includeID bool) bool {
	if includeID && left.ID != right.ID {
		return false
	}
	return left.Zone == right.Zone &&
		left.Name == right.Name &&
		left.FQDN == right.FQDN &&
		left.Type == right.Type &&
		left.Value == right.Value &&
		left.TTL == right.TTL &&
		equalPriority(left.Priority, right.Priority)
}

func equalPriority(left, right *uint16) bool {
	return left == nil && right == nil ||
		left != nil && right != nil && *left == *right
}

func token(environment string, plan Plan, relevant []dns.Record) string {
	relevant = append([]dns.Record(nil), relevant...)
	dns.SortRecords(relevant)
	currentJSON, _ := json.Marshal(relevant)
	requestedJSON, _ := json.Marshal(plan.After)
	sum := sha256.Sum256([]byte(
		schemaVersion + "\x00" + environment + "\x00" + plan.Zone + "\x00" +
			plan.Operation + "\x00" + string(currentJSON) + "\x00" + string(requestedJSON),
	))
	return hex.EncodeToString(sum[:])
}

func recordPointer(record dns.Record) *dns.Record {
	copy := record
	return &copy
}
