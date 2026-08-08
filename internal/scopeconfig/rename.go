package scopeconfig

import (
	"fmt"
	"path/filepath"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"

	"github.com/p3bot/tk/internal/atomicfile"
	"github.com/p3bot/tk/internal/id"
)

// RewriteName rewrites only the top-level name field of <dir>/tk.cue via CUE AST
// (no string templating), preserving other fields, comments, and formatting.
func RewriteName(dir, newName string) error {
	if !id.IsScopeName(newName) {
		return fmt.Errorf("%q is not a legal scope name", newName)
	}
	p := filepath.Join(dir, "tk.cue")
	file, err := parser.ParseFile(p, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", p, err)
	}

	found := false
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if labelName(field.Label) != "name" {
			continue
		}
		newValue := ast.NewString(newName)
		ast.SetComments(newValue, ast.Comments(field.Value))
		field.Value = newValue
		found = true
		break
	}
	if !found {
		return fmt.Errorf("%s has no top-level name field to rewrite", p)
	}

	data, err := format.Node(file)
	if err != nil {
		return fmt.Errorf("format %s: %w", p, err)
	}
	return atomicfile.Write(p, data, 0o600)
}

// labelName returns the string a field label denotes (bare ident or quoted string).
func labelName(label ast.Label) string {
	switch l := label.(type) {
	case *ast.Ident:
		return l.Name
	case *ast.BasicLit:
		if s, err := literal.Unquote(l.Value); err == nil {
			return s
		}
	}
	return ""
}
