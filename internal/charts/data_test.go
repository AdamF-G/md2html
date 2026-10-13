package charts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataLoader(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "docs")
	os.MkdirAll(page, 0o755)
	os.WriteFile(filepath.Join(page, "r.csv"), []byte("k,v\na,1\n"), 0o644)
	os.WriteFile(filepath.Join(t.TempDir(), "secret.csv"), []byte("x"), 0o644)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "o.csv"), []byte("x"), 0o644)
	os.Symlink(filepath.Join(outside, "o.csv"), filepath.Join(page, "link.csv"))
	load := DataLoader(root, page)

	if got, err := load("./r.csv"); err != nil || got != "k,v\na,1\n" {
		t.Errorf("relative file: %q %v", got, err)
	}
	for _, bad := range []string{"https://example.com/r.csv", "//example.com/r.csv", "file:///etc/passwd", "../../etc/passwd", "link.csv", "/etc/passwd"} {
		if _, err := load(bad); err == nil {
			t.Errorf("%s was read", bad)
		} else if !strings.Contains(err.Error(), bad) {
			t.Errorf("error for %s does not name it: %v", bad, err)
		}
	}
	// A file has no query or fragment: md2html would read r.csv and drop
	// the rest without a word, so it refuses instead.
	for _, bad := range []string{"r.csv?v=2", "r.csv#x", "./r.csv?v=2#x"} {
		if _, err := load(bad); err == nil {
			t.Errorf("%s was read", bad)
		} else if !strings.Contains(err.Error(), `"`+bad+`"`) || !strings.Contains(err.Error(), "query or fragment") {
			t.Errorf("error for %s does not name it and its fault: %v", bad, err)
		}
	}
}

// Rendering through the loader: a spec reading a CSV beside the page.
func TestRenderSpecReadsALocalCSV(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "r.csv"), []byte("k,v\na,1\nb,3\n"), 0o644)
	spec := `{"data":{"url":"r.csv"},"mark":"point","encoding":{"x":{"field":"v","type":"quantitative"},"y":{"field":"k","type":"nominal"},"tooltip":{"field":"v"}}}`
	r, err := renderSpec([]byte(spec), fixed{}, DataLoader(root, root))
	if err != nil || len(r.Marks) != 2 {
		t.Fatalf("marks %d err %v", len(r.Marks), err)
	}
}

// A refused or failed read fails the chart with the loader's own reason.
// Vega alone only logs it and draws an empty chart.
func TestRenderSpecFailsOnARefusedRead(t *testing.T) {
	root := t.TempDir()
	spec := `{"data":{"url":"https://example.com/r.csv"},"mark":"point","encoding":{"x":{"field":"v","type":"quantitative"}}}`
	if _, err := renderSpec([]byte(spec), fixed{}, DataLoader(root, root)); err == nil || !strings.Contains(err.Error(), "never the network") {
		t.Errorf("a url with a scheme: err %v", err)
	}
	if _, err := renderSpec([]byte(spec), fixed{}, nil); err == nil || !strings.Contains(err.Error(), "no data files") {
		t.Errorf("no loader: err %v", err)
	}
	// The engine is reused: the next chart is not failed by the last one's read.
	if _, err := renderSpec([]byte(dotSpec), fixed{}, nil); err != nil {
		t.Errorf("the next chart: %v", err)
	}
}

// A url with a query fails the chart like any other refused read.
func TestRenderSpecFailsOnAQuery(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "r.csv"), []byte("k,v\na,1\n"), 0o644)
	spec := `{"data":{"url":"r.csv?v=2"},"mark":"point","encoding":{"x":{"field":"v","type":"quantitative"}}}`
	if _, err := renderSpec([]byte(spec), fixed{}, DataLoader(root, root)); err == nil || !strings.Contains(err.Error(), `"r.csv?v=2"`) {
		t.Errorf("a url with a query: err %v", err)
	}
}
