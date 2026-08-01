package scopeconfig

import (
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/load"
)

// LoadWithClosure evaluates a scope's pj.cue and returns the validated Schema plus
// the import closure (every .cue file the evaluation depends on). Reconcile's eval
// cache keys on the closure's (path, mtime, size). The closure always includes the
// entry pj.cue path even when absent, so creating it later invalidates a cached negative.
func LoadWithClosure(ctx *cue.Context, dir string) (*Schema, []string, error) {
	entry := filepath.Join(dir, "pj.cue")
	if _, err := os.Stat(entry); err != nil {
		if os.IsNotExist(err) {
			return nil, []string{entry}, &ConfigError{Dir: dir, Reason: "pj.cue is absent"}
		}
		return nil, []string{entry}, &ConfigError{Dir: dir, Reason: "cannot read pj.cue: " + err.Error()}
	}

	inst := loadInstance(dir)
	closure := closureFiles(inst, entry)
	if inst.Err != nil {
		return nil, closure, &ConfigError{Dir: dir, Reason: cueReason(inst.Err)}
	}
	v := ctx.BuildInstance(inst)
	schema, err := Evaluate(dir, v)
	return schema, closure, err
}

// loadInstance prefers the directory package (multi-file unify) and falls back to
// the single entry file for package-less minimal configs.
func loadInstance(dir string) *build.Instance {
	cfg := &load.Config{Dir: dir}
	if insts := load.Instances([]string{"."}, cfg); len(insts) > 0 && insts[0].Err == nil {
		return insts[0]
	}
	insts := load.Instances([]string{"./pj.cue"}, cfg)
	if len(insts) == 0 {
		// Loader always returns ≥1 instance for a named file; zero means a CUE-internal break.
		return &build.Instance{Dir: dir}
	}
	return insts[0]
}

// closureFiles collects absolute paths the instance built from (plus entry), deduplicated.
func closureFiles(inst *build.Instance, entry string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(entry)
	if inst != nil {
		for _, f := range inst.BuildFiles {
			add(f.Filename)
		}
		for _, dep := range inst.Imports {
			for _, f := range dep.BuildFiles {
				add(f.Filename)
			}
		}
	}
	return out
}
