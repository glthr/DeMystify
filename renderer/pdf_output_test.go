package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSharedPDFRendererCanPublishToGameScopedOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	fakeNeato(t, "for arg do output=\"$arg\"; done\nprintf 'rendered scoped PDF' > \"$output\"\n")
	output := filepath.Join("generated", "other-game", "graph.pdf")
	if _, err := RenderToFile("other.dot", output, false, false); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(output); err != nil || string(data) != "rendered scoped PDF" {
		t.Fatalf("scoped PDF absent: %q,%v", data, err)
	}
	if _, err := os.Stat(pdfFilePath); !os.IsNotExist(err) {
		t.Fatal("game-scoped output touched the original Myst PDF path")
	}
}
