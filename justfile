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
