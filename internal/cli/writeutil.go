package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/p3bot/pj/internal/atomicfile"
	"github.com/p3bot/pj/internal/frontmatter"
	"github.com/p3bot/pj/internal/index"
	"github.com/p3bot/pj/internal/order"
	"github.com/p3bot/pj/internal/scopeconfig"
	"github.com/p3bot/pj/internal/token"
)

const projectFileMode = 0o644

func atomicWrite(path string, data []byte) error {
	return atomicfile.Write(path, data, projectFileMode)
}

func single(scope, dir string) map[string]string {
	return map[string]string{scope: dir}
}

// writtenPaths includes the removed old path so SyncPaths deletes its row.
func writtenPaths(newPath, oldPath string) []string {
	if oldPath == "" || oldPath == newPath {
		return []string{newPath}
	}
	return []string{newPath, oldPath}
}

// schemaAutoCommit: nil schema is false; writers refuse unusable config first.
func schemaAutoCommit(s *scopeconfig.Schema) bool {
	return s != nil && s.AutoCommit
}

// maxValidOrder: "" means empty board (open KeyBetween bound); skips invalid/quarantine.
func maxValidOrder(rows []*index.Project) string {
	best := ""
	for _, p := range rows {
		if p.ParseError || !order.Valid(p.OrderKey) {
			continue
		}
		if best == "" || p.OrderKey > best {
			best = p.OrderKey
		}
	}
	return best
}

// readProjectFile: parse failure here is a mid-write race (quarantine refused upstream).
func readProjectFile(path string) (*frontmatter.Model, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	interior, body, present := frontmatter.Split(data)
	if !present {
		return nil, nil, fmt.Errorf("%s has no frontmatter fence", path)
	}
	m, err := frontmatter.Parse(interior)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, body, nil
}

func writeProjectFile(path string, m *frontmatter.Model, body []byte) error {
	interior, err := frontmatter.Serialize(m)
	if err != nil {
		return err
	}
	return atomicWrite(path, frontmatter.Compose(interior, body))
}

// resolveSingleRow: 0 → unknown (noun-worded); >1 → duplicate_id; no row-level policy.
func (e *engine) resolveSingleRow(scope, idArg string, form idForm, noun string) (*index.Project, error) {
	var rows []*index.Project
	var err error
	if form == idFull {
		rows, err = e.db.ProjectsByID(scope, idArg)
	} else {
		rows, err = e.db.ProjectsByShortID(scope, idArg)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("unknown %s id %q", noun, idArg)
	}
	if len(rows) > 1 {
		return nil, duplicateRefusal(rows)
	}
	return rows[0], nil
}

// resolveWriteRow also refuses parse_error quarantine.
func (e *engine) resolveWriteRow(scope, idArg string, form idForm) (*index.Project, error) {
	p, err := e.resolveSingleRow(scope, idArg, form, "project")
	if err != nil {
		return nil, err
	}
	if p.ParseError {
		return nil, fmt.Errorf("%s", token.Line(token.ParseError,
			fmt.Sprintf("%s: %s — cannot rewrite quarantined frontmatter", p.ID, p.ParseMsg)))
	}
	return p, nil
}

// terminalLocation relocates only (basename unchanged).
func terminalLocation(dir, base string, terminal bool) (string, error) {
	if !terminal {
		return filepath.Join(dir, base), nil
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", fmt.Errorf("create archive dir: %w", err)
	}
	return filepath.Join(archiveDir, base), nil
}
