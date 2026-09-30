// Package kv reads the KEY=VALUE files updown uses (release.conf, updown.conf):
// one assignment per line, '#' comments, blank lines ignored, no spaces around '=',
// optional matching quotes around the value. The same files are sourced by start.sh,
// so the format stays shell-compatible (docs/contract.md §1).
package kv

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var keyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Parse reads KEY=VALUE lines. A malformed line is an error naming its line number.
func Parse(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || !keyRE.MatchString(key) {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE, got %q", n, line)
		}
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		out[key] = val
	}
	return out, sc.Err()
}

// ParseFile is Parse on a file.
func ParseFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}
