// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestV10MigrationRemovesLegacyUnscopedASSISTCatalogRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	row := json.RawMessage(`{"courseIdentifierParentId":340948,"courseTitle":"Legacy chemistry"}`)
	if err := db.Upsert("courses", "340948", row); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("courses", "cursor", 1); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version=9`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if count, err := db.Count("courses"); err != nil || count != 0 {
		t.Fatalf("legacy course count = %d, err=%v", count, err)
	}
	if hits, err := db.Search("Legacy", 10, "courses"); err != nil || len(hits) != 0 {
		t.Fatalf("legacy FTS hits = %d, err=%v", len(hits), err)
	}
	if _, _, count, err := db.GetSyncState("courses"); err != nil || count != 0 {
		t.Fatalf("legacy sync state count = %d, err=%v", count, err)
	}
}
