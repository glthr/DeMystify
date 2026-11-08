package pdf

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeNeato(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test stub requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "neato"), []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestRenderPDFReturnsSubprocessError(t *testing.T) {
	fakeNeato(t, "printf 'invalid DOT input\\n' >&2\nexit 7\n")
	err := RenderPDF("broken graph.dot")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("expected wrapped process failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid DOT input") {
		t.Fatalf("missing Neato diagnostic: %v", err)
	}
}

func TestRenderPDFPassesPathsAsSingleArguments(t *testing.T) {
	dir := fakeNeato(t, "printf '%s\\n' \"$@\" > \"$NEATO_TEST_ARGS\"\n")
	argsFile := filepath.Join(dir, "args")
	t.Setenv("NEATO_TEST_ARGS", argsFile)
	if err := RenderPDF("a directory/graph.dot"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	expected := "-Tpdf\n-Goverlap=false\n-Gdpi=600\na directory/graph.dot\n-o\n" + pdfFilePath + "\n"
	if string(args) != expected {
		t.Fatalf("unexpected Neato arguments: %q", args)
	}
}
