package scopeconfig

import (
	"fmt"
	"path/filepath"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"

	"github.com/p3bot/pj/internal/atomicfile"
)

// WriteMinimal authors a fresh minimal pj.cue (name + autoCommit) via CUE AST
// and installs it atomically. Init's authoring path; import never writes pj.cue.
func WriteMinimal(dir, name string, autoCommit bool) error {
	file := &ast.File{Decls: []ast.Decl{
		&ast.Field{Label: ast.NewIdent("name"), Value: ast.NewString(name)},
		&ast.Field{Label: ast.NewIdent("autoCommit"), Value: ast.NewBool(autoCommit)},
	}}
	data, err := format.Node(file)
	if err != nil {
		return fmt.Errorf("format pj.cue: %w", err)
	}
	return atomicfile.Write(filepath.Join(dir, "pj.cue"), data, 0o600)
}
