package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOTERecordCRUD(t *testing.T) {
	if os.Getenv("INWX_INTEGRATION") != "1" {
		t.Skip("set INWX_INTEGRATION=1 to run the OT&E integration test")
	}
	if os.Getenv("INWX_ENVIRONMENT") != "ote" {
		t.Fatal("integration tests refuse to run unless INWX_ENVIRONMENT=ote")
	}
	zone := os.Getenv("INWX_TEST_ZONE")
	if zone == "" {
		t.Fatal("INWX_TEST_ZONE must name a disposable OT&E zone")
	}

	options := Options{LookupEnv: os.LookupEnv}
	name := "inwx-cli-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	value := "integration-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	create := []string{
		"--json", "--environment", "ote", "dns", "records", "create", zone,
		"--type", "TXT", "--name", name, "--value", value, "--ttl", "300",
	}
	created := integrationPreviewApply(t, create, options)
	if created.Final == nil || !created.Verified {
		t.Fatalf("create was not verified: %#v", created)
	}
	id := created.Final.ID
	defer func() {
		if id == "" {
			return
		}
		deleteArgs := []string{
			"--json", "--environment", "ote", "dns", "records", "delete", zone, "--id", id,
		}
		preview, err := integrationRun(deleteArgs, options)
		if err != nil {
			t.Errorf("cleanup preview: %v", err)
			return
		}
		apply := append(deleteArgs, "--expect", preview.Expect, "--apply")
		if _, err := integrationRun(apply, options); err != nil {
			t.Errorf("cleanup apply: %v", err)
		}
	}()

	update := []string{
		"--json", "--environment", "ote", "dns", "records", "update", zone,
		"--id", id, "--value", value + "-updated", "--ttl", "600",
	}
	updated := integrationPreviewApply(t, update, options)
	if updated.Final == nil || updated.Final.Value != value+"-updated" || updated.Final.TTL != 600 {
		t.Fatalf("update was not verified: %#v", updated)
	}

	deleteArgs := []string{
		"--json", "--environment", "ote", "dns", "records", "delete", zone, "--id", id,
	}
	deleted := integrationPreviewApply(t, deleteArgs, options)
	if !deleted.Verified || deleted.DeletedID != id {
		t.Fatalf("delete was not verified: %#v", deleted)
	}
	id = ""
}

func integrationPreviewApply(t *testing.T, args []string, options Options) mutationResult {
	t.Helper()
	preview, err := integrationRun(args, options)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.Expect == "" {
		t.Fatalf("invalid preview %#v", preview)
	}
	apply := append(append([]string{}, args...), "--expect", preview.Expect, "--apply")
	result, err := integrationRun(apply, options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func integrationRun(args []string, options Options) (mutationResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	exit := Run(ctx, args, &stdout, &stderr, options)
	if exit != 0 {
		return mutationResult{}, fmt.Errorf("exit %d: %s", exit, strings.TrimSpace(stderr.String()))
	}
	var envelope struct {
		Data mutationResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return mutationResult{}, fmt.Errorf("decode response: %w", err)
	}
	return envelope.Data, nil
}
