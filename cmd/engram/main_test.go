package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateAndListNeedNoProvider(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	input := `{"id":"one","name":"One","statement":"Keep context.","messages":[{"role":"user","content":"raw"}]}`
	var out, diag bytes.Buffer
	if code := run([]string{"create", "--data", data}, strings.NewReader(input), &out, &diag); code != 0 {
		t.Fatalf("create: %s", diag.String())
	}
	out.Reset()
	diag.Reset()
	if code := run([]string{"list", "--data", data}, strings.NewReader(""), &out, &diag); code != 0 || !strings.Contains(out.String(), "one") {
		t.Fatalf("list: %s / %s", out.String(), diag.String())
	}
}
func TestVersion(t *testing.T) {
	var out, diag bytes.Buffer
	if code := run([]string{"version"}, strings.NewReader(""), &out, &diag); code != 0 || out.String() != "0.3.0\n" {
		t.Fatalf("version = %q", out.String())
	}
}

func TestDefaultProviderOutputCeilingSupportsStructuredSelfFold(t *testing.T) {
	if got := defaults().maxOutput; got != 4096 {
		t.Fatalf("default max-output-tokens = %d, want 4096", got)
	}
}
