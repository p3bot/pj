package skill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/p3bot/tk/internal/skill"
)

func TestWriteInstallAndPresent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	written, err := skill.WriteInstall(root)
	if err != nil {
		t.Fatal(err)
	}
	want := skill.FilePath(root)
	if written != want {
		t.Fatalf("written = %q want %q", written, want)
	}
	if !skill.Present(root) {
		t.Fatal("Present = false after install")
	}
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != skill.Text() {
		t.Fatalf("content mismatch: got %d bytes want %d", len(data), len(skill.Text()))
	}
	name, err := skill.FrontmatterName(data)
	if err != nil {
		t.Fatal(err)
	}
	if name != skill.ID {
		t.Fatalf("name = %q want %q", name, skill.ID)
	}
}

func TestWriteInstallRootIsFile(t *testing.T) {
	dir := t.TempDir()
	fileRoot := filepath.Join(dir, "notadir")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.WriteInstall(fileRoot); err == nil {
		t.Fatal("expected error when skills root is a file")
	}
}

func TestWriteInstallRootIsSymlinkToDir(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	written, err := skill.WriteInstall(link)
	if err != nil {
		t.Fatalf("symlink skills root: %v", err)
	}
	// File lands under the real directory via the link path spelling.
	if !skill.Present(link) {
		t.Fatal("Present through symlink root")
	}
	if _, err := os.Stat(filepath.Join(realDir, "tk", "SKILL.md")); err != nil {
		t.Fatalf("expected file under real dir: %v", err)
	}
	if written != skill.FilePath(link) {
		t.Fatalf("written = %q", written)
	}
	res, err := skill.RemoveOwned(link)
	if err != nil || res != skill.UninstallRemoved {
		t.Fatalf("uninstall via symlink root: res=%v err=%v", res, err)
	}
}

func TestRemoveOwned(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if _, err := skill.WriteInstall(root); err != nil {
		t.Fatal(err)
	}
	res, err := skill.RemoveOwned(root)
	if err != nil {
		t.Fatal(err)
	}
	if res != skill.UninstallRemoved {
		t.Fatalf("result = %v want removed", res)
	}
	if skill.Present(root) {
		t.Fatal("still present after remove")
	}
	// absent is OK
	res, err = skill.RemoveOwned(root)
	if err != nil {
		t.Fatal(err)
	}
	if res != skill.UninstallAbsent {
		t.Fatalf("result = %v want absent", res)
	}
}

func TestRemoveOwnedKeepsExtraAndWrongName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if _, err := skill.WriteInstall(root); err != nil {
		t.Fatal(err)
	}
	// Hand-edited body still uninstalls when name is tk.
	path := skill.FilePath(root)
	if err := os.WriteFile(path, []byte("---\nname: tk\n---\n\n# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := skill.RemoveOwned(root)
	if err != nil || res != skill.UninstallRemoved {
		t.Fatalf("edited body: res=%v err=%v", res, err)
	}

	// Re-install then add extra file.
	if _, err := skill.WriteInstall(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill.DirPath(root), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = skill.RemoveOwned(root)
	if err != nil {
		t.Fatal(err)
	}
	if res != skill.UninstallKeptExtra {
		t.Fatalf("extra file: res=%v want kept extra", res)
	}
	if !skill.Present(root) {
		t.Fatal("extra-file dir should remain")
	}

	// Wrong name.
	root2 := filepath.Join(t.TempDir(), "skills2")
	if err := os.MkdirAll(skill.DirPath(root2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill.FilePath(root2), []byte("---\nname: other\n---\n\n# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = skill.RemoveOwned(root2)
	if err != nil {
		t.Fatal(err)
	}
	if res != skill.UninstallKeptNotOurs {
		t.Fatalf("wrong name: res=%v want not ours", res)
	}
	if !skill.Present(root2) {
		t.Fatal("wrong-name dir should remain")
	}

	// Unreadable frontmatter.
	root3 := filepath.Join(t.TempDir(), "skills3")
	if err := os.MkdirAll(skill.DirPath(root3), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill.FilePath(root3), []byte("---\nname: [broken\n---\n\n# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = skill.RemoveOwned(root3)
	if err != nil {
		t.Fatal(err)
	}
	if res != skill.UninstallKeptUnreadable {
		t.Fatalf("broken YAML: res=%v want unreadable", res)
	}
	if skill.ReasonDetail(res) != "skill frontmatter unreadable" {
		t.Fatalf("reason = %q", skill.ReasonDetail(res))
	}
	if !skill.Present(root3) {
		t.Fatal("unreadable dir should remain")
	}
}

func TestDedupeAndJoin(t *testing.T) {
	got := skill.DedupePaths([]string{"/b", "", "/a", "/b", "/a/"})
	// CleanAbs cleans /a/ → /a
	if len(got) != 2 {
		t.Fatalf("dedupe = %v", got)
	}
	if skill.JoinAgents([]string{"z", "a", "m"}) != "a,m,z" {
		t.Fatalf("JoinAgents = %q", skill.JoinAgents([]string{"z", "a", "m"}))
	}
	if !strings.HasPrefix(skill.ReasonDetail(skill.UninstallKeptNotOurs), "frontmatter") {
		t.Fatal("reason detail missing")
	}
}
