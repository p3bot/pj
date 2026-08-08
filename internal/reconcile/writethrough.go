package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
)

// SyncPaths write-throughs specific paths after tk itself wrote them, with no mtime
// skip: the racy-index heuristic is for external edits and can miss same-second
// same-size rewrites of tk's own mutations. Present paths upsert; absent paths delete.
// Touches only the named paths (no dir re-scan, no integrity aggregates).
func (r *Reconciler) SyncPaths(scope string, paths []string) error {
	for _, path := range paths {
		fi, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				if err := r.db.DeleteByPath(path); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("stat %s: %w", path, err)
		}
		fullID, ok := ticketID(scope, filepath.Base(path))
		if !ok {
			// Non-ticket path owns no row to sync.
			continue
		}
		archived := filepath.Base(filepath.Dir(path)) == archiveDir
		p, edges, err := parseFile(path, scope, fullID, archived, fi.ModTime().UnixNano(), fi.Size())
		if err != nil {
			return err
		}
		if err := r.db.UpsertTicketWithEdges(p, edges); err != nil {
			return err
		}
	}
	return nil
}
