package kv

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	m, err := Parse(strings.NewReader("# comment\n\nA=1\nB=\"two words\"\nC='x'\nEMPTY=\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "two words", "C": "x", "EMPTY": ""}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
}

func TestParseRejectsBadLines(t *testing.T) {
	for _, in := range []string{"no equals", "lower=1", "SPACE = 1", "=1"} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%q: want an error", in)
		}
	}
}
