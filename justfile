bindir := env('HOME') / '.local/bin'

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
# That module boundary is the only opt-in: there is no build tag any more.

# Run the browser suite (needs a local Chrome or Chromium).
e2e:
    cd e2e && go test -count=1 ./...
