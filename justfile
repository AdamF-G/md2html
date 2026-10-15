bindir := env('HOME') / '.local/bin'

# The first recipe is the one a bare `just` runs, so this one is first.

# List the recipes.
list:
    @just --list --unsorted

# GOBIN defaults to $HOME/go/bin, which is often not on PATH, so a plain
# `go install` can leave a second binary that nothing ever runs.
#
# The guard is not redundant: `go install` creates a missing GOBIN
# silently, and every level above it, so a typo or a fresh machine
# without ~/.local would get a binary in a directory nothing reads
# and a run that still reported success.

# Build and install md2html to ~/.local/bin, then report its version.
install:
    [ -d '{{bindir}}' ] || { echo 'just: {{bindir}} does not exist - create it first' >&2; exit 1; }
    GOBIN='{{bindir}}' go install ./cmd/md2html
    md2html --version

# Run the library and CLI tests.
test:
    go test ./...

# The browser suite is a separate module, so `go test ./...` at the root
# never reaches it and the library's own go.mod stays free of chromedp.
# That module boundary is the opt-in: there is no build tag any more. The
# compat suite below is separated the same way, for the same reason.

# Run the browser suite (needs a local Chrome or Chromium).
e2e:
    cd e2e && go test -count=1 ./...

# The Pandoc compatibility suite is a separate module for the same reason:
# it needs a tool that is not a dependency of md2html, and `go test ./...`
# at the root must stay runnable with nothing installed.

# Run the Pandoc compatibility suite (needs pandoc; skips without it).
compat:
    cd compat && go test -count=1 ./...

# The built-in grammars are generated, never hand-edited: this reads the
# pinned tm-grammars release and its samples, keeps the permissively
# licensed grammars that match VS Code's own tokenizer on their sample,
# and rewrites plugins/grammars/, the conformance goldens and the
# grammars' notices. It is the only thing that needs Node or the network.

# Regenerate the built-in grammars (needs node, npm and network).
grammars:
    #!/usr/bin/env bash
    set -euo pipefail
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    (cd internal/highlight/gen/node && npm ci --ignore-scripts --no-audit --no-fund)
    pkg=internal/highlight/gen/node/node_modules/tm-grammars
    # The whole archive: tar builds disagree on --wildcards, which a pattern needs.
    curl -sSfL "https://codeload.github.com/shikijs/textmate-grammars-themes/tar.gz/37edd1b26f18838050661d912334aba0ca7f4931" \
        | tar -xz -C "$work"
    mv "$work"/textmate-grammars-themes-*/samples "$work/samples"
    go run ./internal/highlight/gen select -work "$work" -pkg "$pkg"
    node internal/highlight/gen/node/reference.mjs "$work"
    go run ./internal/highlight/gen write -work "$work" -pkg "$pkg" \
        -out plugins/grammars -golden internal/highlight/testdata/conformance -notices THIRD_PARTY_NOTICES

# The documentation site: the repo's own Markdown, wrapped in site/shell.md.
# .github/workflows/pages.yml runs the same command to publish it to GitHub
# Pages, so change the two together.

# Build the documentation site into _site/.
site:
    rm -rf _site
    go run ./cmd/md2html --site site/shell.md --no-source --exclude docs/plans -o _site index.md README.md CHANGELOG.md docs
