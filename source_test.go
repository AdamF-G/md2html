package md2html

import (
	"strings"
	"testing"

	nethtml "golang.org/x/net/html"
)

// embeddedSource parses page as a browser would and returns the embedded
// source element, or nil if the page carries none.
func embeddedSource(t *testing.T, page []byte) *nethtml.Node {
	t.Helper()
	doc, err := nethtml.Parse(strings.NewReader(string(page)))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	var found *nethtml.Node
	walk(doc, func(n *nethtml.Node) {
		if found != nil || n.Type != nethtml.ElementNode {
			return
		}
		if id, _ := attr(n, "id"); id == SourceID {
			found = n
		}
	})
	return found
}

// The point of the embedded copy is that it is the file the author wrote,
// byte for byte, so it can be handed to an agent or run back through
// md2html. Each line of this source is something an HTML parser would
// otherwise change: a leading newline a textarea drops, front matter the
// converter strips, markup and entities it would decode, a closing tag that
// would end the element early, and carriage returns it would normalize.
func TestPageEmbedsSourceVerbatim(t *testing.T) {
	src := "\n---\ntitle: Kept\nauthor: someone\n---\n# Doc\r\n\r\n" +
		"Raw <b>bold</b> &amp; &lt; stays, and so does </textarea> or </TEXTAREA >.\r\n" +
		"Trailing newline too.\n\n"
	page, err := Convert([]byte(src), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	n := embeddedSource(t, page)
	if n == nil {
		t.Fatalf("no #%s element in page:\n%s", SourceID, page)
	}
	if got := textOf(n); got != src {
		t.Errorf("embedded source differs from the original\n got: %q\nwant: %q", got, src)
	}
}

// Hidden so a reader never sees it, and labelled so an agent reading the
// file knows what it has found and which dialect it is written in.
func TestEmbeddedSourceIsHiddenAndLabelled(t *testing.T) {
	page, err := Convert([]byte("# Doc\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	n := embeddedSource(t, page)
	if n == nil {
		t.Fatalf("no #%s element in page", SourceID)
	}
	if n.Data != "textarea" {
		t.Errorf("source element is <%s>, want <textarea>", n.Data)
	}
	if _, ok := attr(n, "hidden"); !ok {
		t.Error("source element is not hidden")
	}
	if got, _ := attr(n, "data-format"); got != "text/markdown" {
		t.Errorf("data-format = %q, want text/markdown", got)
	}
	if got, _ := attr(n, "data-dialect"); got != "github.com/AdamF-G/md2html@"+Version {
		t.Errorf("data-dialect = %q, want github.com/AdamF-G/md2html@%s", got, Version)
	}
	// Outside <main>, so nothing that styles or walks the content sees it.
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == nethtml.ElementNode && p.Data == "main" {
			t.Error("source element is inside <main>")
		}
	}
}

// An agent reading the file from the top learns where the source is before
// it has read the whole rendered page.
func TestPageHeaderPointsAtEmbeddedSource(t *testing.T) {
	page, err := Convert([]byte("# Doc\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	head, _, _ := strings.Cut(string(page), "<!doctype html>")
	if !strings.Contains(head, "#"+SourceID) {
		t.Errorf("nothing ahead of the doctype names #%s:\n%s", SourceID, head)
	}
}

func TestSourceNotEmbedded(t *testing.T) {
	for _, c := range []struct {
		name string
		opt  Options
	}{
		{"opted out", Options{NoSource: true}},
		// An Artifact is published, not handed over as a file, so the
		// embedded copy would not reach the recipient's agent anyway.
		{"fragment", Options{Fragment: true}},
	} {
		page, err := Convert([]byte("# Doc\n"), c.opt)
		if err != nil {
			t.Fatalf("%s: Convert: %v", c.name, err)
		}
		if strings.Contains(string(page), SourceID) {
			t.Errorf("%s: page mentions %s:\n%s", c.name, SourceID, page)
		}
	}
}

// An empty document still has a source to embed: an empty one.
func TestEmptyDocumentEmbedsEmptySource(t *testing.T) {
	page, err := Convert(nil, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	n := embeddedSource(t, page)
	if n == nil {
		t.Fatalf("no #%s element in page", SourceID)
	}
	if got := textOf(n); got != "" {
		t.Errorf("embedded source = %q, want empty", got)
	}
}

// The controls ride with the embedded source and only with it: a page
// without one has nothing for them to copy.
func TestSourceToolsRuntimeFollowsTheSource(t *testing.T) {
	for _, c := range []struct {
		name string
		opt  Options
		want bool
	}{
		{"default", Options{}, true},
		{"opted out", Options{NoSource: true}, false},
		{"fragment", Options{Fragment: true}, false},
	} {
		page, err := Convert([]byte("# Doc\n"), c.opt)
		if err != nil {
			t.Fatalf("%s: Convert: %v", c.name, err)
		}
		// The stylesheet names the controls' classes on every page, so
		// look for the script itself.
		if has := strings.Contains(string(page), sourceToolsRuntime); has != c.want {
			t.Errorf("%s: source tools runtime present = %v, want %v", c.name, has, c.want)
		}
	}
}
