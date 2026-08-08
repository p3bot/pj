package reconcile

import (
	"encoding/json"
	"os"
	"sort"

	"github.com/p3bot/tk/internal/index"
	"github.com/p3bot/tk/internal/scopeconfig"
)

// closureFile is one file in a config import closure with the stat that decides change.
type closureFile struct {
	Path    string `json:"p"`
	MtimeNS int64  `json:"m"`
	Size    int64  `json:"s"`
}

// schemaFor returns a scope's evaluated config, using the index cache when the import
// closure is unchanged. Negative results are cached too. A *ConfigError means the
// config is unusable (reads work; writes blocked).
func (r *Reconciler) schemaFor(scope, dir string) (*scopeconfig.Schema, *scopeconfig.ConfigError) {
	if entry, ok, err := r.db.ConfigCacheGet(scope); err == nil && ok {
		if files, ok := parseClosure(entry.ClosureJSON); ok && closureUnchanged(files) {
			if entry.ConfigError != "" {
				return nil, &scopeconfig.ConfigError{Dir: dir, Reason: entry.ConfigError}
			}
			var s scopeconfig.Schema
			if json.Unmarshal([]byte(entry.SchemaJSON), &s) == nil {
				return &s, nil
			}
		}
	}
	return r.evaluateAndCache(scope, dir)
}

// SchemaCached returns a scope's schema via the cache, or nil when unusable.
// Never reconciles rows or surfaces the config error.
func (r *Reconciler) SchemaCached(scope, dir string) *scopeconfig.Schema {
	s, _ := r.schemaFor(scope, dir)
	return s
}

// SchemaOrError returns a scope's schema or the config error when unusable.
// Never reconciles rows.
func (r *Reconciler) SchemaOrError(scope, dir string) (*scopeconfig.Schema, *scopeconfig.ConfigError) {
	return r.schemaFor(scope, dir)
}

// evaluateAndCache is the cold path: CUE evaluate, stat the closure, store schema or error.
func (r *Reconciler) evaluateAndCache(scope, dir string) (*scopeconfig.Schema, *scopeconfig.ConfigError) {
	schema, closurePaths, loadErr := scopeconfig.LoadWithClosure(r.ctx, dir)
	entry := index.ConfigCacheEntry{ClosureJSON: marshalClosure(statClosure(closurePaths))}

	if loadErr != nil {
		ce, ok := scopeconfig.AsConfigError(loadErr)
		if !ok {
			ce = &scopeconfig.ConfigError{Dir: dir, Reason: loadErr.Error()}
		}
		entry.ConfigError = ce.Reason
		_ = r.db.ConfigCacheSet(scope, entry)
		return nil, ce
	}

	if b, err := json.Marshal(schema); err == nil {
		entry.SchemaJSON = string(b)
	}
	_ = r.db.ConfigCacheSet(scope, entry)
	return schema, nil
}

// statClosure records (mtime, size) per path; unstatable paths get zeroes so later change still invalidates.
func statClosure(paths []string) []closureFile {
	out := make([]closureFile, 0, len(paths))
	for _, p := range paths {
		cf := closureFile{Path: p}
		if fi, err := os.Stat(p); err == nil {
			cf.MtimeNS = fi.ModTime().UnixNano()
			cf.Size = fi.Size()
		}
		out = append(out, cf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// closureUnchanged reports whether every recorded closure file still matches on-disk (mtime, size).
func closureUnchanged(files []closureFile) bool {
	for _, f := range files {
		fi, err := os.Stat(f.Path)
		if err != nil {
			return false
		}
		if fi.ModTime().UnixNano() != f.MtimeNS || fi.Size() != f.Size {
			return false
		}
	}
	return true
}

func marshalClosure(files []closureFile) string {
	b, err := json.Marshal(files)
	if err != nil {
		return ""
	}
	return string(b)
}

func parseClosure(s string) ([]closureFile, bool) {
	if s == "" {
		return nil, false
	}
	var files []closureFile
	if err := json.Unmarshal([]byte(s), &files); err != nil {
		return nil, false
	}
	return files, true
}
