package scopeconfig

import (
	"fmt"
	"path/filepath"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"

	"github.com/p3bot/tk/internal/atomicfile"
)

// WriteMinimal authors a fresh minimal tk.cue (name + autoCommit) via CUE AST
// and installs it atomically. Init's authoring path; import never writes tk.cue.
func WriteMinimal(dir, name string, autoCommit bool) error {
	file := &ast.File{Decls: []ast.Decl{
		&ast.Field{Label: ast.NewIdent("name"), Value: ast.NewString(name)},
		&ast.Field{Label: ast.NewIdent("autoCommit"), Value: ast.NewBool(autoCommit)},
	}}
	data, err := format.Node(file)
	if err != nil {
		return fmt.Errorf("format tk.cue: %w", err)
	}
	return atomicfile.Write(filepath.Join(dir, "tk.cue"), data, 0o600)
}
