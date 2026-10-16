package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// sizeBudget is the stripped binary's ceiling. Stripped builds with
// highlighting measured 19.86 MB on linux/amd64, 20.27 MB on windows/amd64,
// 19.12 MB on darwin/arm64 and 18.75 MB on linux/arm64. The budget comes
// from the largest, windows/amd64, rounded up to 20.5 MB, plus 1.5 MB of
// margin. Raise it deliberately, in a commit that says why.
const sizeBudget = 22 << 20

func TestBinaryStaysUnderItsSizeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	out := filepath.Join(t.TempDir(), "md2html")
	cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", out, ".")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > sizeBudget {
		t.Errorf("binary is %.1f MB, over the %.1f MB budget", float64(fi.Size())/(1<<20), float64(sizeBudget)/(1<<20))
	}
	t.Logf("binary is %.1f MB", float64(fi.Size())/(1<<20))
}
