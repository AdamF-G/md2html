package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A copy is made, made again unchanged, replaced when its source changes,
// and refused when someone else's file, or an edited copy, is in its place.
func TestCopyAssetsKeepsToItsOwnCopies(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.png": "one", "src/b.png": "two", "out/c.png": "theirs"})
	ledger := filepath.Join(root, "out", AssetLedger)
	assets := []Asset{
		{Src: filepath.Join(root, "src/a.png"), Out: filepath.Join(root, "out/img/a.png")},
		{Src: filepath.Join(root, "src/b.png"), Out: filepath.Join(root, "out/c.png")},
	}
	results := func() map[string]WriteResult {
		rs, err := CopyAssets(assets, ledger)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]WriteResult{}
		for _, r := range rs {
			if r.Err != nil {
				t.Fatal(r.Err)
			}
			m[filepath.Base(r.Out)] = r.Result
		}
		return m
	}
	if got := results(); got["a.png"] != WriteCreated || got["c.png"] != WriteRefused {
		t.Errorf("first build: %v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "out/c.png")); string(b) != "theirs" {
		t.Error("someone else's file was overwritten")
	}
	os.WriteFile(filepath.Join(root, "src/a.png"), []byte("one, changed"), 0o644)
	if got := results(); got["a.png"] != WriteOverwritten {
		t.Errorf("changed source: %v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "out/img/a.png")); string(b) != "one, changed" {
		t.Errorf("copy holds %q", b)
	}
	os.WriteFile(filepath.Join(root, "out/img/a.png"), []byte("edited by hand"), 0o644)
	if got := results(); got["a.png"] != WriteRefused {
		t.Errorf("edited copy: %v", got)
	}
	b, err := os.ReadFile(ledger)
	if err != nil || !strings.HasPrefix(string(b), MarkerPrefix) || !strings.Contains(string(b), "img/a.png") {
		t.Errorf("ledger %v:\n%s", err, b)
	}
}

// A copy that already holds its source's bytes is left as it is, even with
// no ledger: a fresh clone of a published site rebuilds cleanly.
func TestCopyAssetsLeavesAnIdenticalFile(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.png": "same", "out/a.png": "same"})
	rs, err := CopyAssets([]Asset{{Src: filepath.Join(root, "src/a.png"), Out: filepath.Join(root, "out/a.png")}},
		filepath.Join(root, "out", AssetLedger))
	if err != nil || rs[0].Result == WriteRefused || rs[0].Err != nil {
		t.Errorf("%+v, %v", rs, err)
	}
}

// A rebuild with nothing changed refuses nothing and leaves the copies'
// bytes as they were.
func TestCopyAssetsUnchangedRebuild(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.png": "one", "src/b.png": "two"})
	ledger := filepath.Join(root, "out", AssetLedger)
	assets := []Asset{
		{Src: filepath.Join(root, "src/a.png"), Out: filepath.Join(root, "out/a.png")},
		{Src: filepath.Join(root, "src/b.png"), Out: filepath.Join(root, "out/img/b.png")},
	}
	if _, err := CopyAssets(assets, ledger); err != nil {
		t.Fatal(err)
	}
	rs, err := CopyAssets(assets, ledger)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.Err != nil || r.Result == WriteRefused {
			t.Errorf("%s: %v, %v", r.Out, r.Result, r.Err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(root, "out/a.png")); string(b) != "one" {
		t.Errorf("a copy holds %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "out/img/b.png")); string(b) != "two" {
		t.Errorf("b copy holds %q", b)
	}
}

// Entries from earlier builds survive a build that no longer lists them:
// a copy stays replaceable, and its ledger line stays while its source is
// gone.
func TestCopyAssetsCarriesEarlierEntries(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.png": "one", "src/b.png": "two"})
	ledger := filepath.Join(root, "out", AssetLedger)
	a := Asset{Src: filepath.Join(root, "src/a.png"), Out: filepath.Join(root, "out/a.png")}
	b := Asset{Src: filepath.Join(root, "src/b.png"), Out: filepath.Join(root, "out/b.png")}
	if _, err := CopyAssets([]Asset{a, b}, ledger); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyAssets([]Asset{a}, ledger); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "src/b.png"), []byte("two, changed"), 0o644)
	rs, err := CopyAssets([]Asset{a, b}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if rs[1].Result != WriteOverwritten || rs[1].Err != nil {
		t.Errorf("b after relinking: %v, %v", rs[1].Result, rs[1].Err)
	}
	// a's source is gone: its copy is kept, and so is its ledger line.
	os.Remove(filepath.Join(root, "src/a.png"))
	if _, err := CopyAssets([]Asset{b}, ledger); err != nil {
		t.Fatal(err)
	}
	lb, err := os.ReadFile(ledger)
	if err != nil || !strings.Contains(string(lb), "  a.png\n") {
		t.Errorf("ledger lost a's line (%v):\n%s", err, lb)
	}
}

// An asset aimed at the ledger's own path is refused, and the ledger is
// not replaced by the asset's bytes.
func TestCopyAssetsRefusesTheLedgerPath(t *testing.T) {
	root := writeTree(t, map[string]string{"src/x.png": "bytes", "out/.keep": ""})
	ledger := filepath.Join(root, "out", AssetLedger)
	rs, err := CopyAssets([]Asset{
		{Src: filepath.Join(root, "src/x.png"), Out: ledger},
		{Src: filepath.Join(root, "src/x.png"), Out: filepath.Join(root, "out/x.png")},
	}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Result != WriteRefused || rs[0].Err == nil || !strings.Contains(rs[0].Err.Error(), "asset ledger") {
		t.Errorf("ledger path: %v, %v", rs[0].Result, rs[0].Err)
	}
	if rs[1].Result != WriteCreated || rs[1].Err != nil {
		t.Errorf("other asset: %v, %v", rs[1].Result, rs[1].Err)
	}
	b, err := os.ReadFile(ledger)
	if err != nil || !strings.HasPrefix(string(b), MarkerPrefix) {
		t.Errorf("ledger %v:\n%s", err, b)
	}
}
