package deployment

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsAndFiltersRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "deployments.json"))
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	if err := store.Add(Record{ID: "one", Site: "alpha", Stage: "build", State: "completed", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Record{ID: "two", Site: "beta", Stage: "activation", State: "completed", CreatedAt: created.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(filepath.Join(filepath.Dir(store.path), "deployments.json"))
	if err != nil {
		t.Fatal(err)
	}
	items := reopened.List("alpha")
	if len(items) != 1 || items[0].ID != "one" {
		t.Fatalf("unexpected filtered records: %#v", items)
	}
	all := reopened.List("")
	if len(all) != 2 || all[0].ID != "two" {
		t.Fatalf("unexpected ordered records: %#v", all)
	}
}

func TestStoreRetainsOriginalStateWhenPersistenceFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "deployments.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Record{ID: "one", Site: "alpha", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	store.path = t.TempDir()
	if err := store.Add(Record{ID: "two", Site: "alpha", CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("expected persistence failure")
	}
	items := store.List("alpha")
	if len(items) != 1 || items[0].ID != "one" {
		t.Fatalf("failed write changed in-memory state: %#v", items)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "control.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE state_blobs (name TEXT PRIMARY KEY, payload BLOB NOT NULL, updated_at INTEGER NOT NULL, revision INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDBStoresSharingHistoryDoNotLoseRecords(t *testing.T) {
	db := openTestDB(t)
	panel, err := OpenDB(db, "")
	if err != nil {
		t.Fatal(err)
	}
	worker, err := OpenDB(db, "")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	if err := panel.Add(Record{ID: "panel", Site: "alpha", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	// The worker's cache predates the panel's record; its write must merge
	// with the durable history instead of replacing it.
	if err := worker.Add(Record{ID: "worker", Site: "alpha", CreatedAt: created.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	for name, store := range map[string]*Store{"panel": panel, "worker": worker} {
		items := store.List("alpha")
		if len(items) != 2 || items[0].ID != "worker" || items[1].ID != "panel" {
			t.Fatalf("%s view of shared history = %#v, want worker then panel", name, items)
		}
	}
}

func TestDBStoreRetainsHistoryWhenWriteFails(t *testing.T) {
	db := openTestDB(t)
	store, err := OpenDB(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Record{ID: "one", Site: "alpha", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject BEFORE UPDATE ON state_blobs BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Record{ID: "two", Site: "alpha", CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if items := store.List("alpha"); len(items) != 1 || items[0].ID != "one" {
		t.Fatalf("failed write changed history: %#v", items)
	}
}
