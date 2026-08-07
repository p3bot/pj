package syncengine

import (
	"context"
	"errors"
	"sort"
	"time"

	"cuelang.org/go/cue"

	"github.com/p3bot/pj/internal/index"
	"github.com/p3bot/pj/internal/integrity"
	"github.com/p3bot/pj/internal/reconcile"
	"github.com/p3bot/pj/internal/registry"
	"github.com/p3bot/pj/internal/scopeconfig"
)

// Deps are machine-local services the sync engine needs.
type Deps struct {
	Ctx      context.Context
	Cue      *cue.Context
	StateDir string
	Reg      *registry.Registry
	DB       *index.DB
	Rec      *reconcile.Reconciler
}

// Reporter receives progress and diagnostic lines (stdout-class Out, stderr-class Err).
type Reporter interface {
	Out(line string)
	Err(line string)
}

// Result is the exit-mapping outcome of a sync run. Progress lines stream on Reporter.
type Result struct {
	// NeedsAttention is true when any root or selection line requires human follow-up.
	NeedsAttention bool
}

// Run selects targets and syncs each git-root. Per-root failures isolate under multi-root.
// Selection-time lines (unreachable, disabled, config, empty-set) and per-root progress
// stream on rep; Result only carries NeedsAttention for process exit mapping.
func Run(deps Deps, rep Reporter, in Input) (Result, error) {
	sel, err := Select(deps, in)
	if err != nil {
		return Result{}, err
	}

	var out Result
	for _, msg := range sel.Unreachable {
		rep.Err(msg)
	}

	if sel.Candidates == 0 {
		if len(sel.Unreachable) > 0 {
			rep.Err("nothing to sync: every registered auto-commit scope is unreachable")
		} else {
			rep.Err("nothing to sync: no auto-commit git-roots registered")
		}
		return out, nil
	}

	for _, msg := range sel.Disabled {
		rep.Err(msg)
		out.NeedsAttention = true
	}
	for _, msg := range sel.ConfigErrs {
		rep.Err(msg)
		out.NeedsAttention = true
	}

	for _, t := range sel.Targets {
		if syncRoot(deps, rep, t) == outcomeNeedsAttention {
			out.NeedsAttention = true
		}
	}
	return out, nil
}

func nowNS() int64 { return time.Now().UnixNano() }

func registeredSet(reg *registry.Registry) map[string]bool {
	out := make(map[string]bool, len(reg.Scopes))
	for name := range reg.Scopes {
		out[name] = true
	}
	return out
}

func sortedRegistered(reg *registry.Registry) []string {
	names := make([]string, 0, len(reg.Scopes))
	for name := range reg.Scopes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func schemaAutoCommit(s *scopeconfig.Schema) bool {
	return s != nil && s.AutoCommit
}

func integrityDeps(deps Deps) integrity.Deps {
	return integrity.Deps{
		Ctx: deps.Ctx, Cue: deps.Cue, StateDir: deps.StateDir,
		Reg: deps.Reg, DB: deps.DB, Rec: deps.Rec,
	}
}

// ErrNeedsAttention is the process-level failure class when roots need human follow-up.
var ErrNeedsAttention = errors.New("pj sync: one or more roots need attention (see the lines above)")
