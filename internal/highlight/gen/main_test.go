package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"strings"
	"testing"
)

// The generated files are committed, so regenerating must not change a
// byte of a grammar that did not change.
func TestGzipIsDeterministic(t *testing.T) {
	in := []byte(`{"scopeName": "source.x", "patterns": []}`)
	a, b := gzipBytes(in), gzipBytes(in)
	if !bytes.Equal(a, b) {
		t.Error("gzipping the same bytes twice gave different output")
	}
	zr, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	if !zr.ModTime.IsZero() || zr.Name != "" {
		t.Errorf("gzip header has time %v and name %q, want neither", zr.ModTime, zr.Name)
	}
}

// The samples commit is written twice, here and in the recipe that
// downloads it; the two must agree.
func TestRecipePinsTheSamplesCommit(t *testing.T) {
	b, err := os.ReadFile("../../../justfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), samplesCommit) {
		t.Errorf("justfile does not download samples commit %s", samplesCommit)
	}
}
