package stylecheck

import (
	"fmt"
	"strings"
)

// Counts says how many findings there are of each severity, worst first,
// leaving out a severity with none: "1 error, 3 warnings, 2 notes".
func Counts(fs []Finding) string {
	var n [3]int
	for _, f := range fs {
		n[f.Severity]++
	}
	var parts []string
	for s, k := range n {
		if k == 0 {
			continue
		}
		word := Severity(s).String()
		if k > 1 {
			word += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", k, word))
	}
	return strings.Join(parts, ", ")
}

// HasError reports whether any finding is an Error.
func HasError(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Report is the full report on one style, as --check-style prints it: a
// heading with the counts, then every finding on its own line.
func Report(name string, fs []Finding) string {
	if len(fs) == 0 {
		return name + ": no problems\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", name, Counts(fs))
	for _, f := range fs {
		fmt.Fprintf(&b, "  %-8s %s\n", f.Severity, f.Message)
	}
	return b.String()
}

// Short is the one-line form a build prints: the findings themselves when
// there are one or two, otherwise their counts. It is "" for none.
func Short(fs []Finding) string {
	if len(fs) > 2 {
		return Counts(fs)
	}
	msgs := make([]string, len(fs))
	for i, f := range fs {
		msgs[i] = f.Message
	}
	return strings.Join(msgs, "; ")
}
