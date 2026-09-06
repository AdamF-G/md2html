package md2html

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// WriteResult reports what SafeWrite did.
type WriteResult int

const (
	// WriteCreated means no file existed at the destination.
	WriteCreated WriteResult = iota
	// WriteOverwritten means an earlier generated file was replaced.
	WriteOverwritten
	// WriteRefused means a file we did not write was left untouched.
	WriteRefused
)

// String implements fmt.Stringer.
func (r WriteResult) String() string {
	switch r {
	case WriteCreated:
		return "created"
	case WriteOverwritten:
		return "overwritten"
	case WriteRefused:
		return "refused"
	}
	return "unknown"
}

// markerWindow is how many leading bytes are searched for the marker.
const markerWindow = 512

// IsOurs reports whether the file at path carries our provenance marker
// within its first 512 bytes. A missing file is not ours, without error.
func IsOurs(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	buf := make([]byte, markerWindow)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, err
	}
	return bytes.Contains(buf[:n], []byte(MarkerPrefix)), nil
}

// SafeWrite writes data to path, refusing to destroy any file this tool did
// not generate. Parent directories are created as needed.
//
// A refusal is a WriteRefused result, not an error: the caller continues
// with other files and reports the refusal in the run summary.
func SafeWrite(path string, data []byte) (WriteResult, error) {
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return WriteRefused, statErr
	}

	if exists {
		ours, err := IsOurs(path)
		if err != nil {
			return WriteRefused, err
		}
		if !ours {
			return WriteRefused, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WriteRefused, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return WriteRefused, err
	}
	if exists {
		return WriteOverwritten, nil
	}
	return WriteCreated, nil
}
