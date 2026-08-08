package index

// SchemaVersion is the on-disk schema version. A mismatch triggers full rebuild (no migrations).
const SchemaVersion = 2

// schemaSQL is the complete DDL for a fresh index. Path is the physical key so
// duplicate ids are two rows and archive moves are delete+insert. FTS rowid mirrors
// tickets.rowid; edges.from_path lets edge delete stay precise under duplicate id.
const schemaSQL = `
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE tickets (
    path            TEXT PRIMARY KEY,
    scope           TEXT NOT NULL,
    id              TEXT NOT NULL,
    short_id        TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT '',
    order_key       TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    created         TEXT NOT NULL DEFAULT '',
    tags            TEXT NOT NULL DEFAULT '',
    custom          TEXT NOT NULL DEFAULT '',
    status_conflict TEXT NOT NULL DEFAULT '',
    archived        INTEGER NOT NULL DEFAULT 0,
    parse_error     INTEGER NOT NULL DEFAULT 0,
    parse_msg       TEXT NOT NULL DEFAULT '',
    schema_error    INTEGER NOT NULL DEFAULT 0,
    mtime_ns        INTEGER NOT NULL DEFAULT 0,
    size            INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_tickets_scope_id ON tickets(scope, id);
CREATE INDEX idx_tickets_scope ON tickets(scope);

CREATE TABLE edges (
    from_path  TEXT NOT NULL,
    from_id    TEXT NOT NULL,
    from_scope TEXT NOT NULL,
    to_id      TEXT NOT NULL,
    to_scope   TEXT NOT NULL,
    kind       TEXT NOT NULL
);
CREATE INDEX idx_edges_from ON edges(from_id);
CREATE INDEX idx_edges_to ON edges(to_id);
CREATE INDEX idx_edges_from_path ON edges(from_path);

CREATE VIRTUAL TABLE fts USING fts5(title, body, tokenize = 'porter unicode61');

CREATE TABLE scope_meta (
    scope      TEXT PRIMARY KEY,
    last_index INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE config_cache (
    scope        TEXT PRIMARY KEY,
    closure_json TEXT NOT NULL DEFAULT '',
    schema_json  TEXT NOT NULL DEFAULT '',
    config_error TEXT NOT NULL DEFAULT ''
);
`

// SchemaText is the human-facing description for tk query --schema (not a stable API).
const SchemaText = `tk index schema (version 2)

NOT A STABLE API: the index is a derived cache, rebuilt on any schema_version
bump, and may reshape between releases with no migration. Do not script against
it — agents use tk deps / list / search / next / get / meta instead.

tickets(path, scope, id, short_id, status, order_key, title, summary, created,
         tags, custom, status_conflict, archived, parse_error, parse_msg,
         schema_error, mtime_ns, size)
    One row per ticket file, keyed by absolute path. tags and custom are JSON;
    status_conflict is a JSON array; archived/parse_error/schema_error are 0/1.

edges(from_path, from_id, from_scope, to_id, to_scope, kind)
    One row per depends/related frontmatter entry (full ids only). kind is
    'depends' or 'related'. Cross-scope edges have from_scope != to_scope; a row
    whose to_id matches no ticket is a dangling edge.

fts(title, body)
    FTS5 corpus; rowid mirrors tickets.rowid. Query with MATCH and rank by
    bm25(fts).

scope_meta(scope, last_index)
    Per-scope last reconcile timestamp (unix nanoseconds).

config_cache(scope, closure_json, schema_json, config_error)
    Cached tk.cue evaluation. closure_json records the (path, mtime, size) of every
    file in the config import closure; a change to any invalidates the cached schema.
`
