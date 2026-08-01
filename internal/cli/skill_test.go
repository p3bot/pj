package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/pj/internal/skill"
	"github.com/start-cli/pj/internal/token"
)

func TestSkillPrintsContractNoScope(t *testing.T) {
	app := newApp(t)
	// Empty config: no registered scopes. skill must still succeed.
	out, errOut, err := run(t, app, "skill")
	if err != nil {
		t.Fatalf("pj skill: %v", err)
	}
	if errOut != "" {
		t.Errorf("skill must write nothing to stderr, got %q", errOut)
	}
	if out != skill.Text() {
		t.Fatalf("stdout is not skill.Text() (got %d bytes, want %d)", len(out), len(skill.Text()))
	}
	for _, h := range skill.RequiredHeadings() {
		if !strings.Contains(out, "## "+h+"\n") {
			t.Errorf("missing section %q", h)
		}
	}
	if entries, _ := os.ReadDir(app.ConfigDir); len(entries) != 0 {
		t.Errorf("skill must not write config dir, found %v", names(entries))
	}
	if entries, _ := os.ReadDir(app.StateDir); len(entries) != 0 {
		t.Errorf("skill must not write state dir, found %v", names(entries))
	}
}

func TestSkillInstallFamilyHardRefuse(t *testing.T) {
	app := newApp(t)
	wantMsg := skillInstallRefuse
	for _, sub := range []string{"install", "list", "uninstall"} {
		t.Run(sub, func(t *testing.T) {
			out, errOut, err := run(t, app, "skill", sub)
			if err == nil {
				t.Fatal("expected non-zero exit")
			}
			if got := ExitCodeFromError(err); got != exitFailure {
				t.Fatalf("exit = %d want %d", got, exitFailure)
			}
			if err.Error() != wantMsg {
				t.Fatalf("message = %q want %q", err.Error(), wantMsg)
			}
			if out != "" {
				t.Errorf("stdout must be empty on refuse, got %q", out)
			}
			var stderr bytes.Buffer
			fprintError(&stderr, err, true)
			if got := strings.TrimRight(stderr.String(), "\n"); got != wantMsg {
				t.Errorf("PrintError path = %q want %q", got, wantMsg)
			}
			if strings.Contains(stderr.String(), "error:") || strings.Contains(stderr.String(), "\x1b") {
				t.Errorf("Plain refuse must not carry label or ANSI: %q", stderr.String())
			}
			if errOut != "" {
				t.Errorf("refuse must write nothing to command stderr, got %q", errOut)
			}
			if entries, _ := os.ReadDir(app.ConfigDir); len(entries) != 0 {
				t.Errorf("refuse must not write config: %v", names(entries))
			}
			if entries, _ := os.ReadDir(app.StateDir); len(entries) != 0 {
				t.Errorf("refuse must not write state dir: %v", names(entries))
			}
		})
	}
}

func TestSkillInstallFamilyInHelp(t *testing.T) {
	app := newApp(t)
	out, _, err := run(t, app, "skill", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, sub := range []string{"install", "list", "uninstall"} {
		if !strings.Contains(out, sub) {
			t.Errorf("skill help missing %q", sub)
		}
	}
}

func TestScopeInitWritesNoAgentsMD(t *testing.T) {
	// scope init must not write AGENTS.md (discovery is skill-driven, not init).
	app := newApp(t)
	dir := filepath.Join(t.TempDir(), "home")
	if _, _, err := run(t, app, "scope", "init", dir, "--name", "home"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("scope init must not write AGENTS.md, stat err=%v", err)
	}
	// Only pj.cue and .gitignore are expected under the scope dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch e.Name() {
		case "pj.cue", ".gitignore":
		default:
			t.Errorf("unexpected init artefact %q", e.Name())
		}
	}
}

func TestSkillDoctorTokensMatchCatalogue(t *testing.T) {
	// Skill body must embed every closed stderr token from token.All().
	out, _, err := run(t, newApp(t), "skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range token.All() {
		if !strings.Contains(out, tok) {
			t.Errorf("skill output missing token %q", tok)
		}
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out
}
