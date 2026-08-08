package index

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sqlite "modernc.org/sqlite"
)

// ErrSearchQuery marks a malformed FTS5 MATCH (the only user-input error Search produces).
var ErrSearchQuery = errors.New("malformed full-text search query")

// sqliteError is SQLITE_ERROR; within Search (static SQL, one MATCH param) it means a bad query.
const sqliteError = 1

// ticketColumns is the fixed list scanTicket expects; Body is never selected.
const ticketColumns = `path, scope, id, short_id, status, order_key, title, summary, created,
    tags, custom, status_conflict, archived, parse_error, parse_msg, schema_error, mtime_ns, size`

func scanTicket(sc interface{ Scan(...any) error }) (*Ticket, error) {
	var (
		p                      Ticket
		tags, custom, conflict string
		archived, perr, serr   int
	)
	if err := sc.Scan(&p.Path, &p.Scope, &p.ID, &p.ShortID, &p.Status, &p.OrderKey, &p.Title, &p.Summary,
		&p.Created, &tags, &custom, &conflict, &archived, &perr, &p.ParseMsg, &serr, &p.MtimeNS, &p.Size); err != nil {
		return nil, err
	}
	p.Archived = archived != 0
	p.ParseError = perr != 0
	p.SchemaError = serr != 0
	if err := unmarshalStrings(tags, &p.Tags); err != nil {
		return nil, err
	}
	if err := unmarshalStrings(conflict, &p.StatusConflict); err != nil {
		return nil, err
	}
	if custom != "" {
		if err := json.Unmarshal([]byte(custom), &p.Custom); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

// AllTickets returns every ticket row machine-wide.
func (d *DB) AllTickets() ([]*Ticket, error) {
	return d.queryTickets(`SELECT ` + ticketColumns + ` FROM tickets`)
}

// ScopeTickets returns every ticket row in one scope.
func (d *DB) ScopeTickets(scope string) ([]*Ticket, error) {
	return d.queryTickets(`SELECT `+ticketColumns+` FROM tickets WHERE scope = ?`, scope)
}

// TicketsByID returns rows in a scope with the given full id (may be >1 under collision).
func (d *DB) TicketsByID(scope, id string) ([]*Ticket, error) {
	return d.queryTickets(`SELECT `+ticketColumns+` FROM tickets WHERE scope = ? AND id = ?`, scope, id)
}

// TicketsByShortID returns rows in a scope with the given short id.
func (d *DB) TicketsByShortID(scope, shortID string) ([]*Ticket, error) {
	return d.queryTickets(`SELECT `+ticketColumns+` FROM tickets WHERE scope = ? AND short_id = ?`, scope, shortID)
}

// TicketsByFullID returns every row machine-wide with the given full id.
func (d *DB) TicketsByFullID(id string) ([]*Ticket, error) {
	return d.queryTickets(`SELECT `+ticketColumns+` FROM tickets WHERE id = ?`, id)
}

func (d *DB) queryTickets(q string, args ...any) ([]*Ticket, error) {
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*Ticket
	for rows.Next() {
		p, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SearchHit is one FTS result with its bm25 score (smaller is better).
type SearchHit struct {
	Ticket *Ticket
	Score  float64
}

// Search runs FTS5 MATCH over titles and bodies (bm25, id tie-break). Empty scope is machine-wide.
func (d *DB) Search(scope, match string) ([]SearchHit, error) {
	q := `SELECT ` + prefixed("p.", ticketColumns) + `, bm25(fts) AS score
          FROM fts JOIN tickets p ON p.rowid = fts.rowid
          WHERE fts MATCH ?`
	args := []any{match}
	if scope != "" {
		q += ` AND p.scope = ?`
		args = append(args, scope)
	}
	q += ` ORDER BY score ASC, p.id ASC`

	rows, err := d.sql.Query(q, args...)
	if err != nil {
		if isQuerySyntaxErr(err) {
			return nil, fmt.Errorf("%w: %w", ErrSearchQuery, err)
		}
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SearchHit
	for rows.Next() {
		hit, err := scanSearchHit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}

// isQuerySyntaxErr reports FTS5 malformed-query errors (SQLITE_ERROR under Search's static SQL).
func isQuerySyntaxErr(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqliteError
}

func scanSearchHit(rows *sql.Rows) (SearchHit, error) {
	var (
		p                      Ticket
		tags, custom, conflict string
		archived, perr, serr   int
		score                  float64
	)
	if err := rows.Scan(&p.Path, &p.Scope, &p.ID, &p.ShortID, &p.Status, &p.OrderKey, &p.Title, &p.Summary,
		&p.Created, &tags, &custom, &conflict, &archived, &perr, &p.ParseMsg, &serr, &p.MtimeNS, &p.Size, &score); err != nil {
		return SearchHit{}, err
	}
	p.Archived = archived != 0
	p.ParseError = perr != 0
	p.SchemaError = serr != 0
	if err := unmarshalStrings(tags, &p.Tags); err != nil {
		return SearchHit{}, err
	}
	if err := unmarshalStrings(conflict, &p.StatusConflict); err != nil {
		return SearchHit{}, err
	}
	if custom != "" {
		if err := json.Unmarshal([]byte(custom), &p.Custom); err != nil {
			return SearchHit{}, err
		}
	}
	return SearchHit{Ticket: &p, Score: score}, nil
}

// AllEdges returns every edge machine-wide.
func (d *DB) AllEdges() ([]Edge, error) {
	return d.queryEdges(`SELECT from_path, from_id, from_scope, to_id, to_scope, kind FROM edges`)
}

// EdgesByTarget returns every edge pointing at toID (ordered for stable reports).
func (d *DB) EdgesByTarget(toID string) ([]Edge, error) {
	return d.queryEdges(`SELECT from_path, from_id, from_scope, to_id, to_scope, kind
	                     FROM edges WHERE to_id = ? ORDER BY from_id, kind`, toID)
}

// EdgesToScope returns every edge whose target lies in toScope (ordered for stable reports).
func (d *DB) EdgesToScope(toScope string) ([]Edge, error) {
	return d.queryEdges(`SELECT from_path, from_id, from_scope, to_id, to_scope, kind
	                     FROM edges WHERE to_scope = ? ORDER BY from_scope, from_id, kind`, toScope)
}

func (d *DB) queryEdges(q string, args ...any) ([]Edge, error) {
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.FromPath, &e.FromID, &e.FromScope, &e.ToID, &e.ToScope, &e.Kind); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func unmarshalStrings(s string, dst *[]string) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), dst)
}

// prefixed rewrites a comma-separated column list so each bare column gets alias (keeps ticketColumns authoritative).
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		trimmed := strings.TrimSpace(c)
		lead := c[:len(c)-len(strings.TrimLeft(c, " \t\n"))]
		parts[i] = lead + alias + trimmed
	}
	return strings.Join(parts, ",")
}
