package pdf

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

func TestRenderPDFReadsFilenameBeginningWithDash(t *testing.T) {
	if _, err := exec.LookPath("neato"); err != nil {
		t.Skip("Neato is not installed")
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("-Tsvg", []byte("digraph G { a -> b }"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RenderPDF("-Tsvg"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pdfFilePath)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("expected PDF from the named DOT file, got %q, %v", data, err)
	}
}
