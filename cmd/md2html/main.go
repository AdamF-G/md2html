// Command md2html converts Markdown documentation trees to HTML.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/AdamF-G/md2html"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// stringList is a flag.Value collecting a repeatable, comma-separable
// option. Both spellings are accepted — "--exclude a --exclude b" and
// "--exclude a,b" — because supporting only one reliably produces a
// directory named "a,b" or a second flag that is silently ignored.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

// run is the testable entry point. It returns the process exit code.
//
// stdout carries only what an invocation was itself asked to report — the
// version, or where the skill went. Document content never goes there:
// md2html always writes files, never streams HTML, and every diagnostic
// goes to stderr, so a future --stdout mode needs no signature change.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("md2html", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, `md2html - convert Markdown docs to HTML

Usage:
  md2html [flags] <entry> [entry...]

Entries may be files or directories, and several may be given in one run:
the emit set is their union, so an extra directory can be built for a
single invocation without being added to a standing entry list. Links
between documents are followed across directories, without limit unless
--link-depth says otherwise, and never into an --exclude'd directory.
Output never leaves -o.

Flags:
`)
		fs.PrintDefaults()
	}

	var (
		outDir    = fs.String("o", "", "output directory (default: write beside each source)")
		depth     = fs.Int("depth", -1, "directory levels to seed from a directory entry (-1 unlimited)")
		linkDepth = fs.Int("link-depth", -1, "hops from a seed that link-following may travel (-1 unlimited, 0 none)")
		fragment  = fs.Bool("fragment", false, "emit Artifact-shaped fragments instead of full pages")
		cssPath   = fs.String("css", "", "replace the embedded stylesheet with this file")
		noTable   = fs.Bool("no-table-scroll", false, "do not wrap tables in a scroll container")
		noAnchor  = fs.Bool("no-anchors", false, "do not add heading anchors")
		noExt     = fs.Bool("no-external-links", false, "do not mark external links")
		noMd      = fs.Bool("no-md-links", false, "do not rewrite .md links")
		noAssets  = fs.Bool("no-assets", false, "do not rewrite asset links")
		version   = fs.Bool("version", false, "print the version and exit")

		installUser    = fs.Bool("install-skill-user", false, "install the authoring skill under ~/.claude/skills and exit")
		installProject = fs.Bool("install-skill-project", false, "install the authoring skill under ./.claude/skills and exit")
	)
	var exclude stringList
	fs.Var(&exclude, "exclude", "directory prefix never to enter or write to (repeatable, comma-separated)")
	// Go's flag package stops parsing at the first positional argument, so a
	// plain fs.Parse would read "md2html ./docs -o ./site" as three entry
	// points. Resume parsing after each positional so flags may appear
	// anywhere, which is what every documented invocation does.
	var entries []string
	for {
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		entries = append(entries, fs.Arg(0))
		args = fs.Args()[1:]
	}
	// Ahead of the entry check, so --version needs no entry point, and
	// ahead of any work, so it never converts a document. The string is
	// the constant stamped into every generated file's provenance marker:
	// what a user reads here always matches what is in their output.
	if *version {
		fmt.Fprintf(stdout, "md2html %s\n", md2html.Version)
		return 0
	}
	// Also ahead of the entry check, and for the same reasons: installing
	// the skill is the whole invocation, not something done alongside a
	// conversion.
	if *installUser || *installProject {
		if *installUser && *installProject {
			fmt.Fprint(stderr, "md2html: give either --install-skill-user or --install-skill-project, not both\n")
			return 2
		}
		return installAuthoringSkill(*installProject, stdout, stderr)
	}
	if len(entries) == 0 {
		fs.Usage()
		return 2
	}

	var css string
	if *cssPath != "" {
		b, err := os.ReadFile(*cssPath)
		if err != nil {
			fmt.Fprintf(stderr, "md2html: --css: %v\n", err)
			return 1
		}
		css = string(b)
	}

	res, err := md2html.Crawl(md2html.CrawlOptions{
		Entries:   entries,
		OutDir:    *outDir,
		Depth:     *depth,
		Exclude:   exclude,
		LinkDepth: *linkDepth,
		NoMdLinks: *noMd,
		NoAssets:  *noAssets,
	})
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 1
	}

	type outcome struct {
		src      string
		res      md2html.WriteResult
		err      error
		warnings []string
	}
	results := make([]outcome, len(res.Docs))

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i := range res.Docs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			d := res.Docs[i]
			src, err := os.ReadFile(d.Src)
			if err != nil {
				results[i] = outcome{src: d.Src, res: md2html.WriteRefused, err: err}
				return
			}
			// One slice per document, written only by this goroutine, so
			// the sink needs no locking and lines from two documents can
			// never interleave.
			var warnings []string
			opts := buildOptions(d, *fragment, css, *noTable, *noAnchor, *noExt,
				func(m string) { warnings = append(warnings, m) })
			out, err := md2html.Convert(src, opts)
			if err != nil {
				results[i] = outcome{src: d.Src, res: md2html.WriteRefused, err: err, warnings: warnings}
				return
			}
			wr, err := md2html.SafeWrite(d.Out, out)
			results[i] = outcome{src: d.Src, res: wr, err: err, warnings: warnings}
		}(i)
	}
	wg.Wait()

	// Per-document conversion warnings are printed here, inside this loop,
	// as each result is visited — ahead of the crawler's own warnings
	// below, which come from a separate pass over res.Warnings and are
	// unrelated to any one document's conversion.
	written, refused, failed, warned := 0, 0, 0, 0
	for i, o := range results {
		for _, w := range o.warnings {
			warned++
			fmt.Fprintf(stderr, "md2html: %s: %s\n", o.src, w)
		}
		switch {
		case o.err != nil:
			failed++
			fmt.Fprintf(stderr, "md2html: %s: %v\n", o.src, o.err)
		case o.res == md2html.WriteRefused:
			refused++
			fmt.Fprintf(stderr, "md2html: refusing to overwrite %s (not generated by md2html)\n",
				res.Docs[i].Out)
		default:
			written++
		}
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "md2html: %s: %s\n", w.Src, w.Message)
	}
	if len(res.External) > 0 {
		fmt.Fprintf(stderr, "md2html: pulled in %d document(s) from outside %s\n",
			len(res.External), res.Base)
		for _, e := range res.External {
			fmt.Fprintf(stderr, "md2html:   %s\n", e)
		}
	}
	fmt.Fprintf(stderr, "md2html: %d written, %d refused, %d failed, %d warning(s)\n",
		written, refused, failed, len(res.Warnings)+warned)

	if refused > 0 || failed > 0 {
		return 1
	}
	return 0
}

// buildOptions assembles per-document conversion options from the flags.
//
// The transform list is built by filtering md2html.Builtins() rather than
// listing constructors by hand, so the CLI's order can never drift from the
// library's — the order is a correctness constraint (Chips before
// HeadingAnchors for slug stability; SectionLinks and TOC after it), and
// md2html_test.go's TestBuiltinsSignatureUnchanged already pins the
// library's copy. main_test.go's TestBuildOptionsMatchesBuiltinsOrder pins
// this one against it directly.
//
// Nothing here wires warn into an individual transform: Options.Warn is the
// single place this run names its sink, and Convert rebuilds the builtins
// that report against it. TestRunReportsContainerWarningToStderr covers the
// one diagnostic that travels that way.
func buildOptions(d md2html.Doc, fragment bool, css string,
	noTable, noAnchor, noExt bool, warn func(string)) md2html.Options {

	skip := map[string]bool{
		"tableScroll":    noTable,
		"headingAnchors": noAnchor,
		"externalLinks":  noExt,
	}
	var ts []md2html.Transform
	for _, t := range md2html.Builtins() {
		if skip[t.Name] {
			continue
		}
		ts = append(ts, t)
	}

	return md2html.Options{
		Fragment:   fragment,
		SourcePath: d.Src,
		CSS:        css,
		Transforms: ts,
		LinkMap:    d.LinkMap,
		Warn:       warn,
	}
}

// installAuthoringSkill writes the embedded authoring skill into a Claude
// Code skills directory: the user's own under ~/.claude, or the caller's
// under ./.claude when project is set.
//
// It will not create the .claude directory itself. A missing one means this
// is not a Claude Code workspace, or the caller is standing somewhere they
// did not mean to be, and inventing it would leave the skill somewhere
// nothing ever reads while still reporting success — the same failure the
// install recipe in the justfile refuses for the binary. skills/ below it
// is ours to create.
//
// Every destination is checked before anything is written, so a refusal
// leaves no half-installed skill: one hand-edited file stops the install
// rather than being stepped around.
func installAuthoringSkill(project bool, stdout, stderr io.Writer) int {
	base := ".claude"
	if !project {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "md2html: --install-skill-user: %v\n", err)
			return 1
		}
		base = filepath.Join(home, ".claude")
	}
	// Resolved before it is reported or written to. The refusal below exists
	// to catch a caller who is not standing where they think they are, and a
	// bare ".claude" is the least useful thing to tell exactly that caller;
	// it also makes both flags report the same shape of path, since the
	// user's is absolute already.
	base, err := filepath.Abs(base)
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 1
	}
	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		fmt.Fprintf(stderr, "md2html: %s does not exist - create it first, "+
			"or run this where it does\n", base)
		return 1
	}

	dir := filepath.Join(base, "skills", md2html.SkillName)
	files := md2html.SkillFiles()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	refused := false
	for _, name := range names {
		ours, err := md2html.IsOurs(filepath.Join(dir, name))
		if err != nil {
			fmt.Fprintf(stderr, "md2html: %s: %v\n", filepath.Join(dir, name), err)
			return 1
		}
		if !ours {
			if _, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil {
				fmt.Fprintf(stderr, "md2html: refusing to overwrite %s (not installed by md2html)\n",
					filepath.Join(dir, name))
				refused = true
			}
		}
	}
	if refused {
		return 1
	}

	for _, name := range names {
		p := filepath.Join(dir, name)
		if _, err := md2html.SafeWrite(p, files[name]); err != nil {
			fmt.Fprintf(stderr, "md2html: %s: %v\n", p, err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "md2html: installed %s %s\n", md2html.SkillName, md2html.Version)
	for _, name := range names {
		fmt.Fprintf(stdout, "md2html:   %s\n", filepath.Join(dir, name))
	}
	return 0
}
