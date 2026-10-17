package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rulesFor reads the rules of a shell at root/site/shell.md, whose front
// matter adds shellFront, for pages under root/docs.
func rulesFor(t *testing.T, shellFront, sourceRootFlag string) (*siteRules, string, error) {
	t.Helper()
	root := writeTree(t, map[string]string{
		"site/shell.md": "---\nsite: true\n" + shellFront + "---\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	opt := CrawlOptions{SiteShell: filepath.Join(root, "site/shell.md"), SourceRoot: sourceRootFlag,
		OutDir: filepath.Join(root, "out")}
	r, err := newSiteRules(opt, filepath.Join(root, "docs"), exclusions{})
	return r, root, err
}

func TestSiteRulesDefaults(t *testing.T) {
	r, root, err := rulesFor(t, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.unmounted != "unlink" || r.sourceRoot != root {
		t.Errorf("unmounted %q, source root %s; want unlink and %s", r.unmounted, r.sourceRoot, root)
	}
	r, _, err = rulesFor(t, "repo-url: https://example.org/r/blob/main/{path}\n", "")
	if err != nil || r.unmounted != "link" {
		t.Errorf("with repo-url: %+v, %v; want unmounted link", r, err)
	}
}

func TestSiteRulesSourceRootKeyAndFlag(t *testing.T) {
	r, root, err := rulesFor(t, "source-root: .\n", "")
	if err != nil || r.sourceRoot != filepath.Join(root, "site") {
		t.Errorf("source-root: . gave %+v, %v; want the shell's directory", r, err)
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	r, _, err = rulesFor(t, "source-root: .\n", dir)
	if err != nil || r.sourceRoot != dir {
		t.Errorf("--source-root gave %+v, %v; want %s, winning over the key", r, err, dir)
	}
}

func TestSiteRulesRefuses(t *testing.T) {
	for front, want := range map[string]string{
		"unmounted: maybe\n":               `unmounted "maybe" is not link, unlink or error`,
		"unmounted: link\n":                "unmounted: link needs repo-url:",
		"repo-url: https://example.org/\n": "repo-url: needs a {path}",
		"source-root: no/such/dir\n":       "source-root:",
	} {
		if _, _, err := rulesFor(t, front, ""); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err %v, want one containing %q", front, err, want)
		}
	}
}

// The default source root is the deepest directory holding the shell and
// the base. When that is the filesystem's root, every path measured from
// it would carry the machine's own, so it is refused.
func TestDefaultSourceRootRefusesTheFilesystemRoot(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("unix paths")
	}
	if got, err := defaultSourceRoot("/repo/site/shell.md", "/repo/docs"); err != nil || got != "/repo" {
		t.Errorf("got %q, %v; want /repo", got, err)
	}
	if _, err := defaultSourceRoot("/repo/site/shell.md", "/tmp/gen"); err == nil || !strings.Contains(err.Error(), "source-root:") {
		t.Errorf("a default at / was not refused: %v", err)
	}
}

// Without a shell, or without an output directory, there are no rules and
// the build is what it was.
func TestSiteRulesOnlyForASiteBuildIntoADirectory(t *testing.T) {
	root := writeTree(t, map[string]string{"site/shell.md": "---\nsite: true\n---\n[[content]]\n"})
	for _, opt := range []CrawlOptions{
		{OutDir: filepath.Join(root, "out")},
		{SiteShell: filepath.Join(root, "site/shell.md")},
	} {
		if r, err := newSiteRules(opt, root, exclusions{}); r != nil || err != nil {
			t.Errorf("%+v: got %+v, %v; want no rules", opt, r, err)
		}
	}
}

// A shell reached through a symlinked directory names its real place, so
// the default and a relative source-root: are measured from the resolved
// shell, not the alias.
func TestSiteRulesShellThroughASymlink(t *testing.T) {
	root := writeTree(t, map[string]string{
		"site/shell.md": "---\nsite: true\n---\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	alias := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Join(root, "site"), alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	opt := CrawlOptions{SiteShell: filepath.Join(alias, "shell.md"), OutDir: filepath.Join(root, "out")}
	r, err := newSiteRules(opt, filepath.Join(root, "docs"), exclusions{})
	if err != nil || r.sourceRoot != root {
		t.Errorf("default through a symlink: %+v, %v; want source root %s", r, err, root)
	}

	if err := os.WriteFile(filepath.Join(root, "site/shell.md"),
		[]byte("---\nsite: true\nsource-root: ..\n---\n[[content]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = newSiteRules(opt, filepath.Join(root, "docs"), exclusions{})
	if err != nil || r.sourceRoot != root {
		t.Errorf("source-root: .. through a symlink: %+v, %v; want source root %s", r, err, root)
	}
}

// A source root that names a file is refused, from the shell's key and
// from the flag alike.
func TestSiteRulesSourceRootNamingAFile(t *testing.T) {
	_, _, err := rulesFor(t, "source-root: shell.md\n", "")
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("source-root: naming a file: err %v, want one containing %q", err, "is not a directory")
	}
	root := writeTree(t, map[string]string{
		"site/shell.md": "---\nsite: true\n---\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	opt := CrawlOptions{SiteShell: filepath.Join(root, "site/shell.md"),
		SourceRoot: filepath.Join(root, "site/shell.md"), OutDir: filepath.Join(root, "out")}
	_, err = newSiteRules(opt, filepath.Join(root, "docs"), exclusions{})
	if err == nil || !strings.Contains(err.Error(), "--source-root") || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("--source-root naming a file: err %v, want one containing %q and %q", err, "--source-root", "is not a directory")
	}
}
