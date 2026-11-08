package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderPDFFailurePreservesPreviousOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("generated", 0755); err != nil {
		t.Fatal(err)
	}
	previous := []byte("previous complete PDF")
	if err := os.WriteFile(pdfFilePath, previous, 0644); err != nil {
		t.Fatal(err)
	}
	fakeNeato(t, "for arg do output=\"$arg\"; done\nprintf 'partial output' > \"$output\"\nexit 7\n")
	if err := RenderPDF("broken.dot"); err == nil {
		t.Fatal("expected subprocess failure")
	}
	got, err := os.ReadFile(pdfFilePath)
	if err != nil || string(got) != string(previous) {
		t.Fatalf("failed render destroyed previous output: %q, %v", got, err)
	}
	leftovers, err := filepath.Glob("generated/.graph-*.pdf")
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary output leaked: %v, %v", leftovers, err)
	}
}
