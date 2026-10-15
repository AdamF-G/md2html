package highlight

import (
	"os/exec"
	"strings"
	"testing"
)

// The engine is swappable because nothing outside this package names it.
func TestOnlyThisPackageImportsTheEngine(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "github.com/AdamF-G/md2html/...").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, " ")
		if strings.Contains(imports, "github.com/eugenioenko/textmate-go") && pkg != "github.com/AdamF-G/md2html/internal/highlight" {
			t.Errorf("%s imports textmate-go; only internal/highlight may", pkg)
		}
	}
}
