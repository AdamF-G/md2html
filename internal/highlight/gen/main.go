// Command gen writes md2html's built-in grammars plugin from a pinned
// tm-grammars release. `just grammars` runs it twice, around the Node
// reference tokenizer:
//
//	select -work DIR -pkg DIR   choose the grammars and write DIR/selected.json
//	write  -work DIR -pkg DIR -out DIR -golden DIR -notices FILE
//	                            check them against DIR/ref and write the plugin
//
// It is never run by go test or by md2html itself.
package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// The pins: the collection's version, and the commit of its source
// repository that released it, which holds the samples. The justfile
// downloads the same commit, and a test holds the two together.
const (
	collection    = "1.32.22"
	samplesCommit = "37edd1b26f18838050661d912334aba0ca7f4931"
)

// allow is the licence allowlist. Anything else ships only through an
// entry in overrides.json.
var allow = map[string]bool{"MIT": true, "Apache-2.0": true, "BSD-2-Clause": true, "BSD-3-Clause": true,
	"ISC": true, "0BSD": true, "Unlicense": true, "CC0-1.0": true}

// langName is md2html's rule for a language name or alias, from
// grammar.go; an alias outside it could never match a fence.
var langName = regexp.MustCompile(`^[a-z][a-z0-9+#.-]*$`)

const (
	beginMarker = "== BEGIN GENERATED GRAMMARS =="
	endMarker   = "== END GENERATED GRAMMARS =="
)

//go:embed overrides.json
var overridesJSON []byte

// overrides are the reviewed exceptions: a licence for a grammar whose
// collection entry has none, and grammars a person decided to leave out.
type overrides struct {
	License map[string]struct {
		SPDX string `json:"spdx"`
		Text string `json:"text"`
		From string `json:"from"`
	} `json:"license"`
	Exclude map[string]string `json:"exclude"`
}

// meta is one grammar's entry in the collection's metadata.
type meta struct {
	Name        string   `json:"name"`
	ScopeName   string   `json:"scopeName"`
	DisplayName string   `json:"displayName"`
	Source      string   `json:"source"`
	License     string   `json:"license"`
	Aliases     []string `json:"aliases"`
}

// selected is one entry of selected.json, which the reference script reads.
type selected struct {
	Name   string `json:"name"`
	Scope  string `json:"scope"`
	Sample string `json:"sample"`
}

func main() {
	if len(os.Args) < 2 {
		fail("usage: gen select|write [flags]")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	work := fs.String("work", "", "the working directory, holding samples/")
	pkg := fs.String("pkg", "", "the installed tm-grammars package")
	out := fs.String("out", "", "the plugin directory to write")
	golden := fs.String("golden", "", "the conformance golden directory to write")
	notices := fs.String("notices", "", "the notices file to update")
	fs.Parse(os.Args[2:])
	if *work == "" || *pkg == "" {
		fail("-work and -pkg are required")
	}
	var ov overrides
	if err := json.Unmarshal(overridesJSON, &ov); err != nil {
		fail("overrides.json: %v", err)
	}
	switch os.Args[1] {
	case "select":
		doSelect(*work, *pkg, ov)
	case "write":
		if *out == "" || *golden == "" || *notices == "" {
			fail("write needs -out, -golden and -notices")
		}
		doWrite(*work, *pkg, *out, *golden, *notices, ov)
	default:
		fail("unknown subcommand %q; want select or write", os.Args[1])
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", a...)
	os.Exit(1)
}

// doSelect reads the collection's metadata, keeps the grammars md2html may
// ship, and writes selected.json for the reference script. It saves the
// metadata as collection.json, so write needs no Node.
func doSelect(work, pkg string, ov overrides) {
	var pj struct {
		Version string `json:"version"`
	}
	b, err := os.ReadFile(filepath.Join(pkg, "package.json"))
	if err == nil {
		err = json.Unmarshal(b, &pj)
	}
	if err != nil {
		fail("reading the collection's package.json: %v", err)
	}
	if pj.Version != collection {
		fail("%s is tm-grammars %s, but the generator is pinned to %s", pkg, pj.Version, collection)
	}

	abs, err := filepath.Abs(filepath.Join(pkg, "index.js"))
	if err != nil {
		fail("%v", err)
	}
	cmd := exec.Command("node", "--input-type=module", "-e",
		fmt.Sprintf("import(%q).then(m => console.log(JSON.stringify(m.grammars)))", "file://"+filepath.ToSlash(abs)))
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		fail("reading the collection's metadata with node: %v", err)
	}
	var all []meta
	if err := json.Unmarshal(raw, &all); err != nil {
		fail("the collection's metadata: %v", err)
	}
	writeJSON(filepath.Join(work, "collection.json"), all)

	kept, left := choose(all, ov)
	var sel []selected
	for _, m := range kept {
		s := selected{Name: m.Name, Scope: m.ScopeName}
		if p := filepath.Join(work, "samples", m.Name+".sample"); exists(p) {
			s.Sample = p
		}
		sel = append(sel, s)
	}
	writeJSON(filepath.Join(work, "selected.json"), sel)
	fmt.Printf("gen: %d grammars kept, %d left out for their licence or by override\n", len(kept), len(left))
}

// leftOut is a grammar that does not ship, and why.
type leftOut struct {
	name, reason string
}

// choose splits the collection into the grammars that may ship and the
// ones that may not, each in name order.
func choose(all []meta, ov overrides) (kept []meta, left []leftOut) {
	for _, m := range all {
		_, override := ov.License[m.Name]
		switch {
		case ov.Exclude[m.Name] != "":
			left = append(left, leftOut{m.Name, "excluded by override: " + ov.Exclude[m.Name]})
		case !allow[m.License] && !override:
			lic := m.License
			if lic == "" {
				lic = "none given"
			}
			left = append(left, leftOut{m.Name, "licence: " + lic})
		default:
			kept = append(kept, m)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	sort.Slice(left, func(i, j int) bool { return left[i].name < left[j].name })
	return kept, left
}

// doWrite checks each selected grammar against the reference dump of its
// sample, then rewrites the plugin, the goldens and the notices from the
// grammars that agree.
func doWrite(work, pkg, out, golden, notices string, ov overrides) {
	var all []meta
	readJSON(filepath.Join(work, "collection.json"), &all)
	kept, left := choose(all, ov)

	raw := map[string][]byte{} // by scope
	for _, m := range kept {
		b, err := os.ReadFile(filepath.Join(pkg, "grammars", m.Name+".json"))
		if err != nil {
			fail("%v", err)
		}
		raw[m.ScopeName] = b
	}
	// Conformance checks correctness, not speed: with the per-fence budget,
	// a slow machine could leave a grammar out and change the output.
	e := highlight.NewTextMate(func(scope string) ([]byte, error) { return raw[scope], nil }, highlight.Options{Budget: time.Minute})

	// Every grammar is checked against the same set the reference loaded,
	// so one that fails is dropped only after all are checked.
	refs := map[string]highlight.Dump{}
	failed := map[string]string{}
	diags := map[string][]string{}
	for _, m := range kept {
		sample := filepath.Join(work, "samples", m.Name+".sample")
		if !exists(sample) {
			continue
		}
		var want highlight.Dump
		readJSON(filepath.Join(work, "ref", m.Name+".json"), &want)
		got, err := highlight.DumpOf(e, m.Name, m.ScopeName, want.Lines)
		if err != nil {
			failed[m.Name] = fmt.Sprintf("md2html could not tokenize the sample: %v", err)
			continue
		}
		if d := highlight.Compare(want, got); d != "" {
			failed[m.Name] = d
			continue
		}
		refs[m.Name] = want
		diags[m.Name] = e.Diagnostics(m.ScopeName)
	}

	var ship []meta
	for _, m := range kept {
		if why, ok := failed[m.Name]; ok {
			left = append(left, leftOut{m.Name, "conformance: " + why})
			continue
		}
		ship = append(ship, m)
	}
	sort.Slice(left, func(i, j int) bool { return left[i].name < left[j].name })
	for _, m := range kept {
		if _, ok := failed[m.Name]; !ok {
			continue
		}
		inc := regexp.MustCompile(`"include"\s*:\s*"` + regexp.QuoteMeta(m.ScopeName) + `(#[^"]*)?"`)
		for _, k := range ship {
			if inc.Match(raw[k.ScopeName]) {
				fail("%s fails conformance but %s, which ships, includes it; add %s to exclude in overrides.json and rerun",
					m.Name, k.Name, m.Name)
			}
		}
	}

	notice := parseNotice(filepath.Join(pkg, "NOTICE"))
	writePlugin(out, ship, raw, ov)
	writeLeftOut(filepath.Join(out, "left-out.txt"), all, ship, left)
	writeReport(filepath.Join(out, "REPORT.md"), ship, left, diags, refs)
	writeGoldens(golden, ship, refs)
	writeNotices(notices, filepath.Join(pkg, "LICENSE"), ship, notice, ov)
	fmt.Printf("gen: %d grammars written, %d left out\n", len(ship), len(left))
}

// writePlugin replaces dir with the gzipped grammars and their manifest.
func writePlugin(dir string, ship []meta, raw map[string][]byte, ov overrides) {
	resetDir(dir)
	type entry struct {
		File    string   `json:"file"`
		Scope   string   `json:"scope"`
		License string   `json:"license"`
		Source  string   `json:"source"`
		Aliases []string `json:"aliases,omitempty"`
	}
	grammars := map[string]entry{} // json.Marshal sorts map keys
	for _, m := range ship {
		file := m.Name + ".json.gz"
		writeFile(filepath.Join(dir, file), gzipBytes(raw[m.ScopeName]))
		grammars[m.Name] = entry{File: file, Scope: m.ScopeName, License: licence(m, ov), Source: m.Source, Aliases: aliases(m)}
	}
	manifest := struct {
		Format      int              `json:"format"`
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Grammars    map[string]entry `json:"grammars"`
	}{1, "grammars", "Languages for highlighting code, from tm-grammars " + collection, grammars}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fail("%v", err)
	}
	writeFile(filepath.Join(dir, "md2html-plugin.json"), append(b, '\n'))
}

// licence is the SPDX identifier m ships under.
func licence(m meta, ov overrides) string {
	if o, ok := ov.License[m.Name]; ok {
		return o.SPDX
	}
	return m.License
}

// aliases are m's aliases that a fence can name; the rest are reported.
func aliases(m meta) []string {
	var out []string
	for _, a := range m.Aliases {
		if langName.MatchString(a) {
			out = append(out, a)
		}
	}
	return out
}

// writeLeftOut lists, one to a line, the names and aliases of the
// grammars that do not ship. They are real languages, not typos, so
// md2html leaves their fences plain without suggesting a near name. A name
// a shipped grammar also claims is left off, as that grammar serves it.
func writeLeftOut(path string, all, ship []meta, left []leftOut) {
	byName := map[string]meta{}
	for _, m := range all {
		byName[m.Name] = m
	}
	shipped := map[string]bool{}
	for _, m := range ship {
		shipped[m.Name] = true
		for _, a := range aliases(m) {
			shipped[a] = true
		}
	}
	seen := map[string]bool{}
	var names []string
	for _, l := range left {
		m := byName[l.name]
		for _, n := range append([]string{m.Name}, m.Aliases...) {
			n = strings.ToLower(n)
			if langName.MatchString(n) && !shipped[n] && !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	writeFile(path, []byte(strings.Join(names, "\n")+"\n"))
}

func writeReport(path string, ship []meta, left []leftOut, diags map[string][]string, refs map[string]highlight.Dump) {
	var b strings.Builder
	fmt.Fprintf(&b, "# The built-in grammars\n\nGenerated by `just grammars`; do not edit.\n\n")
	fmt.Fprintf(&b, "- Collection: tm-grammars %s\n- Samples: shikijs/textmate-grammars-themes at %s\n\n", collection, samplesCommit)

	fmt.Fprintf(&b, "## Kept (%d)\n\n", len(ship))
	for _, m := range ship {
		note := ""
		if _, ok := refs[m.Name]; !ok {
			note = " (no sample, so not checked for conformance)"
		}
		fmt.Fprintf(&b, "- %s%s\n", m.Name, note)
	}

	fmt.Fprintf(&b, "\n## Left out (%d)\n\n", len(left))
	for _, l := range left {
		fmt.Fprintf(&b, "- %s: %s\n", l.name, l.reason)
	}

	fmt.Fprintf(&b, "\n## Aliases no fence can name\n\n")
	n := 0
	for _, m := range ship {
		for _, a := range m.Aliases {
			if !langName.MatchString(a) {
				fmt.Fprintf(&b, "- %s: %q is not lowercase letters, digits, -, +, # and . starting with a letter\n", m.Name, a)
				n++
			}
		}
	}
	if n == 0 {
		b.WriteString("None.\n")
	}

	fmt.Fprintf(&b, "\n## Untranslatable patterns\n\nThe patterns each grammar's sample reached that md2html's engine could not\nuse. A long pattern is cut in the middle, keeping its start and the reason.\n\n")
	n = 0
	for _, m := range ship {
		if len(diags[m.Name]) == 0 {
			continue
		}
		n++
		fmt.Fprintf(&b, "### %s\n\n", m.Name)
		for _, d := range diags[m.Name] {
			fmt.Fprintf(&b, "- `%s`\n", strings.ReplaceAll(highlight.Abbreviate(d), "`", "'"))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		b.WriteString("None.\n")
	}
	writeFile(path, []byte(strings.TrimRight(b.String(), "\n")+"\n"))
}

// writeGoldens replaces dir with each shipped grammar's reference dump.
func writeGoldens(dir string, ship []meta, refs map[string]highlight.Dump) {
	resetDir(dir)
	for _, m := range ship {
		d, ok := refs[m.Name]
		if !ok {
			continue
		}
		b, err := json.Marshal(d)
		if err != nil {
			fail("%v", err)
		}
		writeFile(filepath.Join(dir, m.Name+".json.gz"), gzipBytes(b))
	}
}

// parseNotice maps each grammar file named in the collection's NOTICE to
// the licence text given for it.
func parseNotice(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		fail("%v", err)
	}
	// A block's header is a rule of equals signs, Files, License and SPDX
	// lines, and a dashed rule. Licence texts contain rules of their own,
	// so a block runs to the next full header.
	head := regexp.MustCompile(`(?m)^={20,}\nFiles:\s*([^\n]*)\nLicense:[^\n]*\nSPDX:[^\n]*\n-{20,}\n`)
	text := string(b)
	at := head.FindAllStringSubmatchIndex(text, -1)
	if len(at) == 0 {
		fail("NOTICE: no blocks found")
	}
	out := map[string]string{}
	for i, m := range at {
		end := len(text)
		if i+1 < len(at) {
			end = at[i+1][0]
		}
		body := strings.TrimRight(strings.TrimLeft(text[m[1]:end], "\n"), " \t\n")
		for _, f := range strings.Split(text[m[2]:m[3]], ",") {
			out[strings.TrimSpace(f)] = body
		}
	}
	return out
}

// writeNotices replaces the generated section of the notices file. Shipped
// grammars that share a licence text, as the many from one editor's
// repository do, share one copy of it under their headings.
func writeNotices(path, collectionLicense string, ship []meta, notice map[string]string, ov overrides) {
	cur, err := os.ReadFile(path)
	if err != nil {
		fail("%v", err)
	}
	text := string(cur)
	if !strings.Contains(text, beginMarker) || !strings.Contains(text, endMarker) {
		text = strings.TrimRight(text, "\n") + "\n\n" + beginMarker + "\n" + endMarker + "\n"
	}
	before := text[:strings.Index(text, beginMarker)+len(beginMarker)]
	after := text[strings.Index(text, endMarker):]

	type group struct {
		heads []string
		text  string
	}
	var groups []*group
	byText := map[string]*group{}
	for _, m := range ship {
		t, src := notice[m.Name+".json"], m.Source
		if o, ok := ov.License[m.Name]; ok {
			t = o.Text + "\n\n(From " + o.From + ")"
		}
		if t == "" {
			fail("the collection's NOTICE has no licence text for %s", m.Name)
		}
		name := m.DisplayName
		if name == "" {
			name = m.Name
		}
		head := fmt.Sprintf("== %s grammar (%s) ==\n\nSource: %s", name, licence(m, ov), src)
		g := byText[t]
		if g == nil {
			g = &group{text: t}
			byText[t] = g
			groups = append(groups, g)
		}
		g.heads = append(g.heads, head)
	}

	lic, err := os.ReadFile(collectionLicense)
	if err != nil {
		fail("%v", err)
	}
	var b strings.Builder
	b.WriteString(before)
	fmt.Fprintf(&b, "\n\nThe built-in grammars plugin carries these TextMate grammars, each from\nthe source named, as collected by tm-grammars %s.\n", collection)
	for _, g := range groups {
		for _, h := range g.heads {
			b.WriteString("\n" + h + "\n")
		}
		b.WriteString("\n" + g.text + "\n")
	}
	fmt.Fprintf(&b, "\n== tm-grammars %s (MIT) ==\n\n%s\n\n", collection, strings.TrimSpace(string(lic)))
	b.WriteString(after)
	writeFile(path, []byte(b.String()))
}

// gzipBytes compresses b with no name and no time in the header, so the
// same input always gives the same bytes.
func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		fail("%v", err)
	}
	if _, err := zw.Write(b); err != nil {
		fail("%v", err)
	}
	if err := zw.Close(); err != nil {
		fail("%v", err)
	}
	return buf.Bytes()
}

func resetDir(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		fail("%v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail("%v", err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readJSON(path string, v any) {
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		fail("%s: %v", path, err)
	}
}

func writeJSON(path string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fail("%v", err)
	}
	writeFile(path, b)
}

func writeFile(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fail("%v", err)
	}
}
