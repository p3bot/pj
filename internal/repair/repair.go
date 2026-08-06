// Package repair holds deterministic, bit-identical integrity-repair procedures:
// id-collision loser pick and short-id extension, equal-order and over-long-order
// re-space, and archive-layout move. No crypto/rand, dirent order, mtime, or pointer
// identity enters a decision. Returns rewrite.Op values only — never writes files.
package repair

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/p3bot/pj/internal/collision"
	"github.com/p3bot/pj/internal/frontmatter"
	"github.com/p3bot/pj/internal/id"
	"github.com/p3bot/pj/internal/order"
	"github.com/p3bot/pj/internal/rewrite"
)

// OrderLongThreshold is the soft length above which an order key is eligible for re-space.
const OrderLongThreshold = 64

// Row is the minimal projection of an indexed project a repair procedure needs.
type Row struct {
	Path       string
	FullID     string
	ShortID    string
	OrderKey   string
	ParseError bool
}

// Rename records one collision-loser rename for the operation's report and commit message.
type Rename struct {
	OldID   string
	NewID   string
	OldPath string
	NewPath string
}

// member is one file in a same-id collision for the deterministic loser pick.
type member struct {
	path     string
	basename string
	created  string
	shortID  string
	raw      []byte
	model    *frontmatter.Model
	body     []byte
}

// DuplicateID builds ops that resolve one duplicate-id collision: keep the
// deterministically chosen member, rename every other by short-id extension.
// occupied maps short-ids to holding paths (caller-owned; extended with each mint)
// so re-entry can recognise a loser a crashed prior run already extended.
// Edges on losers are left untouched because the kept side retains the original id.
func DuplicateID(scope string, rows []Row, occupied map[string]string) ([]rewrite.Op, []Rename, error) {
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("duplicate-id repair needs at least two members, got %d", len(rows))
	}
	members := make([]member, 0, len(rows))
	for _, r := range rows {
		m, err := readMember(r)
		if err != nil {
			return nil, nil, err
		}
		members = append(members, m)
	}
	// Kept member sorts first; losers follow.
	sort.SliceStable(members, func(i, j int) bool { return keepBefore(members[i], members[j]) })

	taken := make(map[string]struct{}, len(occupied))
	for short := range occupied {
		taken[short] = struct{}{}
	}

	var ops []rewrite.Op
	var renames []Rename
	for _, loser := range members[1:] {
		op, rn, resumed, err := resumeExtension(loser, scope, occupied)
		if err != nil {
			return nil, nil, err
		}
		if resumed {
			ops = append(ops, op)
			renames = append(renames, rn)
			continue
		}
		newShort, err := id.Extend(loser.shortID, taken)
		if err != nil {
			return nil, nil, fmt.Errorf("collision repair for %s: %w (files %s)", loser.model.ID, err, membersPaths(members))
		}
		taken[newShort] = struct{}{}
		newID := scope + "-" + newShort
		newPath := filepath.Join(filepath.Dir(loser.path), Basename(loser.basename, newID))

		m := *loser.model
		m.ID = newID
		content, err := serialize(&m, loser.body)
		if err != nil {
			return nil, nil, err
		}
		occupied[newShort] = newPath
		ops = append(ops, rewrite.Op{OldPath: loser.path, NewPath: newPath, Content: content})
		renames = append(renames, Rename{OldID: loser.model.ID, NewID: newID, OldPath: loser.path, NewPath: newPath})
	}
	return ops, renames, nil
}

// resumeExtension recognises a loser already extended by a crashed prior run.
// Recognition is by content modulo id (extension rewrites the id, so not byte-identical).
func resumeExtension(loser member, scope string, occupied map[string]string) (rewrite.Op, Rename, bool, error) {
	var candidates []string
	for short := range occupied {
		if len(short) > len(loser.shortID) && strings.HasPrefix(short, loser.shortID) {
			candidates = append(candidates, short)
		}
	}
	sort.Strings(candidates)

	for _, short := range candidates {
		newID := scope + "-" + short
		m := *loser.model
		m.ID = newID
		content, err := serialize(&m, loser.body)
		if err != nil {
			return rewrite.Op{}, Rename{}, false, err
		}
		path := occupied[short]
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return rewrite.Op{}, Rename{}, false, fmt.Errorf("read %s: %w", path, err)
		}
		if !bytes.Equal(raw, content) {
			continue
		}
		return rewrite.Op{OldPath: loser.path, NewPath: path, Content: content},
			Rename{OldID: loser.model.ID, NewID: newID, OldPath: loser.path, NewPath: path}, true, nil
	}
	return rewrite.Op{}, Rename{}, false, nil
}

// EqualOrder builds ops that re-space equal (tied) order keys, preserving (order, id) relative order.
func EqualOrder(rows []Row) ([]rewrite.Op, error) {
	valid := validOrderRows(rows)
	counts := map[string]int{}
	for _, r := range valid {
		counts[r.OrderKey]++
	}
	return respace(valid, func(r Row) bool { return counts[r.OrderKey] > 1 })
}

// LongOrder builds ops that re-space pathologically long order keys into shorter legal keys.
func LongOrder(rows []Row) ([]rewrite.Op, error) {
	valid := validOrderRows(rows)
	return respace(valid, func(r Row) bool { return len(r.OrderKey) > OrderLongThreshold })
}

// ArchiveMove builds the op that relocates a project file across the archive boundary
// to match terminal-ness. Frontmatter is unchanged (byte-for-byte preservation).
func ArchiveMove(dir string, row Row, terminal bool) (rewrite.Op, error) {
	raw, err := os.ReadFile(row.Path)
	if err != nil {
		return rewrite.Op{}, fmt.Errorf("read %s: %w", row.Path, err)
	}
	base := filepath.Base(row.Path)
	newPath := filepath.Join(dir, base)
	if terminal {
		newPath = filepath.Join(dir, "archive", base)
	}
	return rewrite.Op{OldPath: row.Path, NewPath: newPath, Content: raw}, nil
}

// InterruptedMove reports whether a same-id set is the both-present window of an
// interrupted archive-layout move: two byte-identical copies, one at dir root and one
// under archive/. Extending a short-id here would fork one project into two ids.
func InterruptedMove(dir string, rows []Row) (bool, error) {
	archiveDir := filepath.Join(dir, "archive")
	var atRoot, archived []Row
	for _, r := range rows {
		switch filepath.Dir(r.Path) {
		case dir:
			atRoot = append(atRoot, r)
		case archiveDir:
			archived = append(archived, r)
		}
	}
	for _, a := range atRoot {
		for _, b := range archived {
			same, err := sameContent(a.Path, b.Path)
			if err != nil {
				return false, err
			}
			if same {
				return true, nil
			}
		}
	}
	return false, nil
}

func sameContent(a, b string) (bool, error) {
	ra, err := os.ReadFile(a)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", a, err)
	}
	rb, err := os.ReadFile(b)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", b, err)
	}
	return bytes.Equal(ra, rb), nil
}

// respace assigns distinct legal keys to rows needing rewrite, minting each between
// the running previous key and the nearest untied right anchor.
func respace(valid []Row, needsRewrite func(Row) bool) ([]rewrite.Op, error) {
	sort.SliceStable(valid, func(i, j int) bool {
		if valid[i].OrderKey != valid[j].OrderKey {
			return valid[i].OrderKey < valid[j].OrderKey
		}
		return valid[i].FullID < valid[j].FullID
	})
	rewriteAt := make([]bool, len(valid))
	for i, r := range valid {
		rewriteAt[i] = needsRewrite(r)
	}

	var ops []rewrite.Op
	prev := ""
	for i, r := range valid {
		if !rewriteAt[i] {
			prev = r.OrderKey
			continue
		}
		right := ""
		for j := i + 1; j < len(valid); j++ {
			if !rewriteAt[j] {
				right = valid[j].OrderKey
				break
			}
		}
		newKey, err := order.KeyBetween(prev, right)
		if err != nil {
			return nil, fmt.Errorf("re-space order for %s between %q and %q: %w", r.FullID, prev, right, err)
		}
		prev = newKey
		if newKey == r.OrderKey {
			continue
		}
		op, err := orderRewriteOp(r.Path, newKey)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// orderRewriteOp rewrites only the order key of a project file as an in-place op.
func orderRewriteOp(path, newKey string) (rewrite.Op, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rewrite.Op{}, fmt.Errorf("read %s: %w", path, err)
	}
	interior, body, present := frontmatter.Split(raw)
	if !present {
		return rewrite.Op{}, fmt.Errorf("%s has no frontmatter fence", path)
	}
	m, err := frontmatter.Parse(interior)
	if err != nil {
		return rewrite.Op{}, fmt.Errorf("parse %s: %w", path, err)
	}
	m.Order = newKey
	content, err := serialize(m, body)
	if err != nil {
		return rewrite.Op{}, err
	}
	return rewrite.Op{OldPath: path, NewPath: path, Content: content}, nil
}

// readMember reads one collision member for the loser pick and id rewrite.
func readMember(r Row) (member, error) {
	raw, err := os.ReadFile(r.Path)
	if err != nil {
		return member{}, fmt.Errorf("read %s: %w", r.Path, err)
	}
	interior, body, present := frontmatter.Split(raw)
	if !present {
		return member{}, fmt.Errorf("%s has no frontmatter fence", r.Path)
	}
	m, err := frontmatter.Parse(interior)
	if err != nil {
		return member{}, fmt.Errorf("parse %s: %w", r.Path, err)
	}
	return member{
		path:     r.Path,
		basename: filepath.Base(r.Path),
		created:  m.Created,
		shortID:  r.ShortID,
		raw:      raw,
		model:    m,
		body:     body,
	}, nil
}

// keepBefore adapts disk-backed members onto collision.KeepBefore.
func keepBefore(a, b member) bool {
	return collision.KeepBefore(a.toCollision(), b.toCollision())
}

func (m member) toCollision() collision.Member {
	return collision.Member{Created: m.created, Basename: m.basename, Raw: m.raw, Path: m.path}
}

// Basename is the project filename for newID that preserves base's frozen slug.
// Never consults the old id, because filename and frontmatter id can disagree;
// scope names and short-ids contain no hyphen, so the first two segments are the id.
func Basename(base, newID string) string {
	stem := strings.TrimSuffix(base, ".md")
	parts := strings.SplitN(stem, "-", 3)
	if len(parts) < 3 || parts[2] == "" {
		return newID + ".md"
	}
	return newID + "-" + parts[2] + ".md"
}

func serialize(m *frontmatter.Model, body []byte) ([]byte, error) {
	interior, err := frontmatter.Serialize(m)
	if err != nil {
		return nil, err
	}
	return frontmatter.Compose(interior, body), nil
}

func validOrderRows(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if !r.ParseError && order.Valid(r.OrderKey) {
			out = append(out, r)
		}
	}
	return out
}

func membersPaths(members []member) string {
	paths := make([]string, len(members))
	for i, m := range members {
		paths[i] = m.path
	}
	return strings.Join(paths, ", ")
}
