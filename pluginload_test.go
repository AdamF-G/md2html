package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pluginDir writes a plugin's files under a fresh temporary directory and
// returns that directory, with symlinks resolved so paths compare equal.
func pluginDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, c := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const acmeManifest = `{"format": 1, "name": "acme", "styles": {"plain": {"css": ["style.css"]}}}`

func TestLoadFromDisk(t *testing.T) {
	root := pluginDir(t, map[string]string{"p/md2html-plugin.json": acmeManifest, "p/style.css": "a{}"})
	p, err := NewPluginCache().Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "acme" || p.dir != filepath.Join(root, "p") {
		t.Errorf("loaded %q from %q", p.Name, p.dir)
	}
}

func TestLoadRelativeToBaseDirNotWorkingDir(t *testing.T) {
	root := pluginDir(t, map[string]string{"docs/_style/md2html-plugin.json": acmeManifest, "docs/_style/style.css": ""})
	if _, err := NewPluginCache().Load("./_style", filepath.Join(root, "docs"), nil); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRefuses(t *testing.T) {
	root := pluginDir(t, map[string]string{"empty/x": "", "bad/md2html-plugin.json": `{`})
	for _, c := range []struct{ dir, want string }{
		{"missing", "no such file"},
		{"empty", "no md2html-plugin.json"},
		{"bad", "not valid JSON"},
		{"empty/x", "not a directory"},
	} {
		_, err := NewPluginCache().Load(c.dir, root, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", c.dir, err, c.want)
		}
	}
}

// Two spellings of one directory, and a symlink to it, are one plugin: one
// cache entry, the same *Plugin.
func TestLoadSameDirectoryIsOnePlugin(t *testing.T) {
	root := pluginDir(t, map[string]string{"p/md2html-plugin.json": acmeManifest, "p/style.css": ""})
	if err := os.Symlink(filepath.Join(root, "p"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	c := NewPluginCache()
	a, _ := c.Load("p", root, nil)
	b, _ := c.Load("./p/../p", root, nil)
	l, _ := c.Load("link", root, nil)
	if a == nil || a != b || a != l {
		t.Errorf("want one *Plugin, got %p %p %p", a, b, l)
	}
}

func TestLoadReplaysWarningsEachCall(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "acme", "extra": 1, "styles": {"s": {"css": ["s.css"]}}}`,
	})
	c := NewPluginCache()
	for i := 0; i < 2; i++ {
		var got []string
		if _, err := c.Load("p", root, func(w string) { got = append(got, w) }); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !strings.Contains(got[0], `unknown key "extra"`) {
			t.Errorf("call %d: warnings = %q", i, got)
		}
	}
}

func TestReadFileStaysInsideThePlugin(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": acmeManifest, "p/ok.woff2": "F", "secret.woff2": "S",
	})
	if err := os.Symlink(filepath.Join(root, "secret.woff2"), filepath.Join(root, "p", "out.woff2")); err != nil {
		t.Fatal(err)
	}
	p, err := NewPluginCache().Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := p.readFile("ok.woff2"); err != nil || string(b) != "F" {
		t.Errorf("ok.woff2: %q, %v", b, err)
	}
	for _, rel := range []string{"../secret.woff2", "out.woff2", "/etc/hosts"} {
		if _, err := p.readFile(rel); err == nil || !strings.Contains(err.Error(), "outside the plugin") {
			t.Errorf("%s: err = %v, want outside the plugin", rel, err)
		}
	}
}

func TestBuiltinPluginsIncludeDefault(t *testing.T) {
	for _, p := range BuiltinPlugins() {
		if p.dir != "" || p.key != "builtin:"+p.Name {
			t.Errorf("%s: dir %q key %q", p.Name, p.dir, p.key)
		}
	}
	def := findPlugin(BuiltinPlugins(), "default")
	if def == nil {
		t.Fatal("no built-in default plugin")
	}
	b, err := def.readFile("default.css")
	if err != nil || string(b) != defaultCSS {
		t.Errorf("default plugin's default.css differs from defaultCSS (err %v)", err)
	}
}

func TestAvailablePluginsRefusesClashes(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"a/md2html-plugin.json": acmeManifest,
		"b/md2html-plugin.json": acmeManifest,
	})
	c := NewPluginCache()
	a, _ := c.Load("a", root, nil)
	b, _ := c.Load("b", root, nil)
	if _, err := AvailablePlugins([]*Plugin{a, a}); err != nil {
		t.Errorf("one plugin twice: %v", err)
	}
	_, err := AvailablePlugins([]*Plugin{a, b})
	if err == nil || !strings.Contains(err.Error(), `both named "acme"`) {
		t.Errorf("clash: err = %v", err)
	}
}

func TestAvailablePluginsRefusesShadowingABuiltin(t *testing.T) {
	builtin := BuiltinPlugins()[0]
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "` + builtin.Name + `", "styles": {"s": {"css": ["s.css"]}}}`,
	})
	p, err := NewPluginCache().Load("p", root, nil)
	if builtin.Name == "default" {
		// The reserved name is refused at parse time instead.
		if err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("err = %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AvailablePlugins([]*Plugin{p}); err == nil {
		t.Error("a local plugin shadowed a built-in")
	}
}
