package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

type mutableRecord struct {
	ID      string
	Name    string
	Type    string
	Content string
	TTL     int
	Prio    int
}

type mutationServer struct {
	t                  *testing.T
	server             *httptest.Server
	mutex              sync.Mutex
	records            []mutableRecord
	nextID             int
	mutations          int
	loseCreateResponse bool
	skipCreate         bool
}

func newMutationServer(t *testing.T) *mutationServer {
	state := &mutationServer{
		t:      t,
		nextID: 100,
		records: []mutableRecord{{
			ID: "1", Name: "keep.example.test", Type: "TXT", Content: "unchanged", TTL: 3600,
		}},
	}
	state.server = httptest.NewServer(http.HandlerFunc(state.handle))
	return state
}

func (state *mutationServer) handle(writer http.ResponseWriter, request *http.Request) {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	var call struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
		state.t.Error(err)
		return
	}
	switch call.Method {
	case "account.login":
		writeCLIEnvelope(state.t, writer, map[string]string{"tfa": "0"})
	case "account.logout":
		writeCLIEnvelope(state.t, writer, struct{}{})
	case "nameserver.info":
		raw := make([]map[string]any, 0, len(state.records))
		for _, record := range state.records {
			raw = append(raw, map[string]any{
				"id": record.ID, "name": record.Name, "type": record.Type,
				"content": record.Content, "ttl": record.TTL, "prio": record.Prio,
			})
		}
		writeCLIEnvelope(state.t, writer, map[string]any{"count": len(raw), "record": raw})
	case "nameserver.createRecord":
		state.nextID++
		id := strconv.Itoa(state.nextID)
		if !state.skipCreate {
			state.records = append(state.records, mutableFromParams(id, call.Params))
		}
		state.mutations++
		if state.loseCreateResponse {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeCLIEnvelope(state.t, writer, map[string]string{"id": id})
	case "nameserver.updateRecord":
		id := strconv.Itoa(int(call.Params["id"].(float64)))
		for index := range state.records {
			if state.records[index].ID == id {
				state.records[index] = mutableFromParams(id, call.Params)
			}
		}
		state.mutations++
		writeCLIEnvelope(state.t, writer, struct{}{})
	case "nameserver.deleteRecord":
		id := strconv.Itoa(int(call.Params["id"].(float64)))
		filtered := state.records[:0]
		for _, record := range state.records {
			if record.ID != id {
				filtered = append(filtered, record)
			}
		}
		state.records = filtered
		state.mutations++
		writeCLIEnvelope(state.t, writer, struct{}{})
	default:
		state.t.Errorf("unexpected method %q", call.Method)
	}
}

func mutableFromParams(id string, params map[string]any) mutableRecord {
	return mutableRecord{
		ID:      id,
		Name:    params["name"].(string),
		Type:    params["type"].(string),
		Content: params["content"].(string),
		TTL:     int(params["ttl"].(float64)),
		Prio:    int(params["prio"].(float64)),
	}
}

func TestRecordCRUDRequiresPreviewAndPreservesUnrelatedRecord(t *testing.T) {
	state := newMutationServer(t)
	defer state.server.Close()
	options := testOptions(state.server.URL)

	create := []string{
		"--json", "--environment", "ote", "dns", "records", "create", "example.test",
		"--type", "A", "--name", "www", "--value", "192.0.2.1",
	}
	createPreview := runMutationJSON(t, create, options)
	if createPreview.Applied || state.mutations != 0 {
		t.Fatal("create preview mutated state")
	}
	createApply := append(append([]string{}, create...), "--expect", createPreview.Expect, "--apply")
	created := runMutationJSON(t, createApply, options)
	if !created.Applied || !created.Verified || created.Final == nil || state.mutations != 1 {
		t.Fatalf("create not verified: %#v mutations=%d", created, state.mutations)
	}

	update := []string{
		"--json", "--environment", "ote", "dns", "records", "update", "example.test",
		"--id", created.Final.ID, "--value", "192.0.2.2", "--ttl", "7200",
	}
	updatePreview := runMutationJSON(t, update, options)
	updateApply := append(append([]string{}, update...), "--expect", updatePreview.Expect, "--apply")
	updated := runMutationJSON(t, updateApply, options)
	if !updated.Verified || updated.Final.Value != "192.0.2.2" || updated.Final.TTL != 7200 {
		t.Fatalf("update not verified: %#v", updated)
	}

	deleteArgs := []string{
		"--json", "--environment", "ote", "dns", "records", "delete", "example.test",
		"--id", created.Final.ID,
	}
	deletePreview := runMutationJSON(t, deleteArgs, options)
	deleteApply := append(append([]string{}, deleteArgs...), "--expect", deletePreview.Expect, "--apply")
	deleted := runMutationJSON(t, deleteApply, options)
	if !deleted.Verified || deleted.DeletedID != created.Final.ID || state.mutations != 3 {
		t.Fatalf("delete not verified: %#v mutations=%d", deleted, state.mutations)
	}

	state.mutex.Lock()
	defer state.mutex.Unlock()
	if len(state.records) != 1 || state.records[0].ID != "1" || state.records[0].Content != "unchanged" {
		t.Fatalf("unrelated record changed: %#v", state.records)
	}
}

func TestStaleExpectAndDuplicateFailWithoutMutation(t *testing.T) {
	state := newMutationServer(t)
	defer state.server.Close()
	options := testOptions(state.server.URL)
	args := []string{
		"--json", "--environment", "ote", "dns", "records", "create", "example.test",
		"--type", "A", "--name", "stale", "--value", "192.0.2.1",
	}
	preview := runMutationJSON(t, args, options)
	state.mutex.Lock()
	state.records = append(state.records, mutableRecord{
		ID: "50", Name: "stale.example.test", Type: "TXT", Content: "changed", TTL: 3600,
	})
	state.mutex.Unlock()

	var stdout, stderr bytes.Buffer
	apply := append(append([]string{}, args...), "--expect", preview.Expect, "--apply")
	exit := Run(context.Background(), apply, &stdout, &stderr, options)
	if exit != 5 || stdout.Len() != 0 || state.mutations != 0 {
		t.Fatalf("stale apply exit=%d stdout=%q stderr=%q mutations=%d", exit, stdout.String(), stderr.String(), state.mutations)
	}

	state.mutex.Lock()
	state.records = append(state.records, mutableRecord{
		ID: "51", Name: "duplicate.example.test", Type: "A", Content: "192.0.2.1", TTL: 3600,
	})
	state.mutex.Unlock()
	duplicate := []string{
		"--json", "--environment", "ote", "dns", "records", "create", "example.test",
		"--type", "A", "--name", "duplicate", "--value", "192.0.2.1",
	}
	stdout.Reset()
	stderr.Reset()
	if exit := Run(context.Background(), duplicate, &stdout, &stderr, options); exit != 5 || state.mutations != 0 {
		t.Fatalf("duplicate exit=%d stderr=%q mutations=%d", exit, stderr.String(), state.mutations)
	}
}

func TestApplyRequiresExpectAndUnsupportedDeleteIsProtected(t *testing.T) {
	state := newMutationServer(t)
	defer state.server.Close()
	options := testOptions(state.server.URL)

	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{
		"--environment", "ote", "dns", "records", "create", "example.test",
		"--type", "TXT", "--name", "www", "--value", "value", "--apply",
	}, &stdout, &stderr, options)
	if exit != 2 || state.mutations != 0 {
		t.Fatalf("missing expect exit=%d mutations=%d", exit, state.mutations)
	}

	state.mutex.Lock()
	state.records = append(state.records, mutableRecord{
		ID: "60", Name: "example.test", Type: "NS", Content: "ns.example.test", TTL: 3600,
	})
	state.mutex.Unlock()
	exit = Run(context.Background(), []string{
		"--environment", "ote", "dns", "records", "delete", "example.test", "--id", "60",
	}, &stdout, &stderr, options)
	if exit != 5 || state.mutations != 0 {
		t.Fatalf("protected delete exit=%d mutations=%d", exit, state.mutations)
	}
}

func TestLostCreateResponseIsRecoveredWithoutRetry(t *testing.T) {
	state := newMutationServer(t)
	defer state.server.Close()
	state.loseCreateResponse = true
	options := testOptions(state.server.URL)
	args := []string{
		"--json", "--environment", "ote", "--retries", "5",
		"dns", "records", "create", "example.test",
		"--type", "TXT", "--name", "recovery", "--value", "value",
	}
	preview := runMutationJSON(t, args, options)
	apply := append(append([]string{}, args...), "--expect", preview.Expect, "--apply")
	result := runMutationJSON(t, apply, options)
	if !result.Applied || !result.Verified || !result.Recovered || state.mutations != 1 {
		t.Fatalf("lost response was not safely recovered: %#v mutations=%d", result, state.mutations)
	}
}

func TestSuccessfulResponseWithoutStateFailsVerification(t *testing.T) {
	state := newMutationServer(t)
	defer state.server.Close()
	state.skipCreate = true
	options := testOptions(state.server.URL)
	args := []string{
		"--json", "--environment", "ote", "dns", "records", "create", "example.test",
		"--type", "TXT", "--name", "missing", "--value", "value",
	}
	preview := runMutationJSON(t, args, options)
	apply := append(append([]string{}, args...), "--expect", preview.Expect, "--apply")
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), apply, &stdout, &stderr, options)
	if exit != 6 || stdout.Len() != 0 || state.mutations != 1 {
		t.Fatalf("verification exit=%d stdout=%q stderr=%q mutations=%d", exit, stdout.String(), stderr.String(), state.mutations)
	}
}

func TestCanceledMutationCommandNeverWrites(t *testing.T) {
	state := newMutationServer(t)
	defer state.server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	exit := Run(ctx, []string{
		"--environment", "ote", "dns", "records", "create", "example.test",
		"--type", "TXT", "--name", "canceled", "--value", "value",
	}, &stdout, &stderr, testOptions(state.server.URL))
	if exit != 130 || state.mutations != 0 {
		t.Fatalf("canceled exit=%d mutations=%d", exit, state.mutations)
	}
}

type mutationResult struct {
	Expect    string      `json:"expect"`
	Applied   bool        `json:"applied"`
	Verified  bool        `json:"verified"`
	Recovered bool        `json:"recovered"`
	DeletedID string      `json:"deleted_id"`
	Final     *recordJSON `json:"final"`
}

type recordJSON struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

func runMutationJSON(t *testing.T, args []string, options Options) mutationResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), args, &stdout, &stderr, options)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var envelope struct {
		Data mutationResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", stdout.String(), err)
	}
	return envelope.Data
}
