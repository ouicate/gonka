package storage

import (
	"context"
	"decentralized-api/apiconfig"
	"decentralized-api/utils"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*apiconfig.SqliteDb, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db := apiconfig.NewSQLiteDb(apiconfig.SqliteConfig{Path: dbPath})
	if err := db.BootstrapLocal(context.Background()); err != nil {
		t.Fatalf("bootstrap sqlite: %v", err)
	}
	cleanup := func() {
		if db.GetDb() != nil {
			_ = db.GetDb().Close()
		}
		_ = os.RemoveAll(dir)
	}
	return db, cleanup
}

func TestPromptPayloadCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	now := time.Now()
	canonical, err := utils.CanonicalizeJSON([]byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	rec := PromptPayloadRecord{
		InferenceID:      "inf-1",
		PromptPayload:    canonical,
		PromptHash:       utils.GenerateSHA256Hash(canonical),
		Model:            "test-model",
		RequestTimestamp: now.UnixNano(),
		StoredBy:         "transfer",
	}

	// Save
	if err := SavePromptPayload(context.Background(), db.GetDb(), rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Get
	got, ok, err := GetPromptPayload(context.Background(), db.GetDb(), rec.InferenceID)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.PromptPayload != rec.PromptPayload || got.PromptHash != rec.PromptHash || got.Model != rec.Model {
		t.Fatalf("mismatch: %+v", got)
	}

	// Upsert update
	rec2 := rec
	rec2.StoredBy = "executor"
	if err := SavePromptPayload(context.Background(), db.GetDb(), rec2); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got2, ok, err := GetPromptPayload(context.Background(), db.GetDb(), rec.InferenceID)
	if err != nil || !ok {
		t.Fatalf("get2: ok=%v err=%v", ok, err)
	}
	if got2.StoredBy != "executor" {
		t.Fatalf("expected stored_by executor, got %s", got2.StoredBy)
	}

	// Prune older than future (should delete)
	cutoff := time.Now().Add(24 * time.Hour)
	n, err := DeletePromptPayloadsOlderThan(context.Background(), db.GetDb(), cutoff)
	if err != nil {
		t.Fatalf("delete older than: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected deletions, got %d", n)
	}

	// Ensure gone
	_, ok, err = GetPromptPayload(context.Background(), db.GetDb(), rec.InferenceID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatalf("expected no record after delete")
	}
}
