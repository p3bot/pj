package index

import (
	"fmt"
	"sort"
	"strings"
)

// Collision is a group of project rows in one scope that share a key that must be
// unique (full id or order key). Members are sorted paths for stable warnings.
type Collision struct {
	Scope   string
	Key     string
	Members []string
}

// DuplicateIDs returns full ids claimed by two or more files in the given scopes.
func (d *DB) DuplicateIDs(scopes []string) ([]Collision, error) {
	return d.collisions(scopes, "id", `1`)
}

// EqualOrders returns non-empty order keys shared by two or more projects in the given scopes.
func (d *DB) EqualOrders(scopes []string) ([]Collision, error) {
	return d.collisions(scopes, "order_key", `order_key <> ''`)
}

// collisions groups rows by keyCol, keeping groups of size > 1.
func (d *DB) collisions(scopes []string, keyCol, extraPred string) ([]Collision, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	placeholders, args := inClause(scopes)
	q := fmt.Sprintf(`SELECT scope, %s AS k, path FROM projects
                      WHERE scope IN (%s) AND %s
                      ORDER BY scope, k, path`, keyCol, placeholders, extraPred)
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("collision aggregate: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type key struct{ scope, k string }
	grouped := map[key][]string{}
	var order []key
	for rows.Next() {
		var scope, k, path string
		if err := rows.Scan(&scope, &k, &path); err != nil {
			return nil, err
		}
		kk := key{scope, k}
		if _, seen := grouped[kk]; !seen {
			order = append(order, kk)
		}
		grouped[kk] = append(grouped[kk], path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []Collision
	for _, kk := range order {
		members := grouped[kk]
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		out = append(out, Collision{Scope: kk.scope, Key: kk.k, Members: members})
	}
	return out, nil
}

// ParseErrorCount returns how many parse_error quarantine rows exist across scopes.
func (d *DB) ParseErrorCount(scopes []string) (int, error) {
	if len(scopes) == 0 {
		return 0, nil
	}
	placeholders, args := inClause(scopes)
	var n int
	err := d.sql.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM projects WHERE parse_error = 1 AND scope IN (%s)`, placeholders), args...).Scan(&n)
	return n, err
}

// DuplicateIDSet returns scope-qualified collision keys ("<scope>\x00<id>") for next's skip set.
func (d *DB) DuplicateIDSet(scopes []string) (map[string]bool, error) {
	cols, err := d.DuplicateIDs(scopes)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, c := range cols {
		out[c.Scope+"\x00"+c.Key] = true
	}
	return out, nil
}

func inClause(values []string) (string, []any) {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	marks := make([]string, len(sorted))
	args := make([]any, len(sorted))
	for i, v := range sorted {
		marks[i] = "?"
		args[i] = v
	}
	return strings.Join(marks, ", "), args
}
