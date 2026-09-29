package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
)

// page writes text to stdout, through a pager when stdout is a terminal.
// The whole guide runs to a thousand lines, and a terminal given them all at
// once shows only the end. Anything else — a pipe, a file, a test buffer,
// an agent capturing output — gets the bytes unchanged, so --guide stays
// safe to read straight into a context.
//
// The pager is chosen the way git chooses one: $PAGER, else less, and
// "cat" or an empty $PAGER means none. If the pager cannot be started the
// text is written directly rather than lost.
func page(stdout io.Writer, text []byte) {
	f, ok := stdout.(*os.File)
	if !ok || !isTerminal(f) {
		stdout.Write(text)
		return
	}
	argv := pagerCommand(os.LookupEnv)
	if argv == nil {
		stdout.Write(text)
		return
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(string(text))
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	// Like git: quit if it fits on one screen, pass colour through, and
	// leave the text on the terminal afterwards. Only when LESS is unset,
	// so a reader's own less options win.
	if _, set := os.LookupEnv("LESS"); !set {
		cmd.Env = append(os.Environ(), "LESS=FRX")
	}
	if err := cmd.Start(); err != nil {
		stdout.Write(text)
		return
	}
	// The reader quitting early is not an error worth reporting.
	cmd.Wait()
}

// pagerCommand returns the pager to run, or nil for none. An unset $PAGER
// means less; a set but empty one, or "cat", means the reader asked for
// no pager.
func pagerCommand(lookup func(string) (string, bool)) []string {
	v, set := lookup("PAGER")
	if !set {
		return []string{"less"}
	}
	argv := strings.Fields(v)
	if len(argv) == 0 || argv[0] == "cat" {
		return nil
	}
	return argv
}

// isTerminal reports whether f is a character device, which for stdout
// means a terminal. It avoids a dependency for the one check; the only
// other character device stdout is commonly bound to is /dev/null, where
// less writes straight through without prompting.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
