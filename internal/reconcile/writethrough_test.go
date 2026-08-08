package reconcile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// SyncPaths must reflect a write tk just made regardless of mtime.
func TestSyncPathsUpsertsAndDeletes(t *testing.T) {
	r, db := newReconciler(t)
	dir := mkScope(t, "wc")
	fp := filepath.Join(dir, "wc-ab2c-x.md")
	writeFile(t, fp, projFile("wc-ab2c", "todo", "a0", "# X\n"))

	now := time.Now().UnixNano()
	reconcileOne(t, r, "wc", dir, now)

	// Same-size status rewrite with mtime pinned: read-path reconcile would skip; SyncPaths must not.
	writeFile(t, fp, projFile("wc-ab2c", "done", "a0", "# X\n"))
	if err := os.Chtimes(fp, time.Unix(0, now), time.Unix(0, now)); err != nil {
		t.Fatal(err)
	}
	if err := r.SyncPaths("wc", []string{fp}); err != nil {
		t.Fatalf("SyncPaths upsert: %v", err)
	}
	rows, _ := db.ScopeTickets("wc")
	if len(rows) != 1 || rows[0].Status != "done" {
		t.Fatalf("SyncPaths must upsert the new state regardless of mtime, got %+v", rows)
	}

	if err := os.Remove(fp); err != nil {
		t.Fatal(err)
	}
	if err := r.SyncPaths("wc", []string{fp}); err != nil {
		t.Fatalf("SyncPaths delete: %v", err)
	}
	rows, _ = db.ScopeTickets("wc")
	if len(rows) != 0 {
		t.Fatalf("SyncPaths must delete a removed path's row, got %+v", rows)
	}
}
