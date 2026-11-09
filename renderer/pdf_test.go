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
	t.Chdir(t.TempDir())
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
	t.Chdir(t.TempDir())
	dir := fakeNeato(t, "printf '%s\\n' \"$@\" > \"$NEATO_TEST_ARGS\"\nfor arg do output=\"$arg\"; done\nprintf 'rendered PDF' > \"$output\"\n")
	argsFile := filepath.Join(dir, "args")
	t.Setenv("NEATO_TEST_ARGS", argsFile)
	if err := RenderPDF("a directory/graph.dot"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	input, err := filepath.Abs("a directory/graph.dot")
	if err != nil {
		t.Fatal(err)
	}
	expected := "-Tpdf\n-Goverlap=false\n-Gdpi=600\n" + input + "\n-o\n"
	arguments := strings.Split(strings.TrimSuffix(string(args), "\n"), "\n")
	if !strings.HasPrefix(string(args), expected) || len(arguments) != 6 || filepath.Dir(arguments[5]) != filepath.Dir(pdfFilePath) || arguments[5] == pdfFilePath {
		t.Fatalf("unexpected Neato arguments: %q", args)
	}
	if result, err := os.ReadFile(pdfFilePath); err != nil || string(result) != "rendered PDF" {
		t.Fatalf("rendered PDF was not installed: %q, %v", result, err)
	}
}
