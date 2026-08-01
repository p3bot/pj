package cli

import (
	"strings"

	"github.com/start-cli/pj/internal/id"
)

type idForm int

const (
	idFull idForm = iota
	idShort
)

// parseIDArg: malformed → ok false (caller → exit 2); unknown well-formed is lookup's exit 1.
func parseIDArg(tok string) (idForm, bool) {
	if strings.ContainsRune(tok, '-') {
		return idFull, id.IsFullProjectID(tok)
	}
	return idShort, id.IsShortID(tok)
}
