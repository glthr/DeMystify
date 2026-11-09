package pdf

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const pdfFilePath = "generated/graph.pdf"

// RenderPDF generates a PDF visualization using Neato
// NOTE: it would be great to replace this system call with a function call (library)
func RenderPDF(dotFilePath string) error {
	// Graphviz does not accept "--"; absolute paths keep leading dashes literal.
	dotInputPath, err := filepath.Abs(dotFilePath)
	if err != nil {
		return fmt.Errorf("resolving DOT input path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pdfFilePath), 0755); err != nil {
		return fmt.Errorf("creating PDF output directory: %w", err)
	}
	output, err := os.CreateTemp(filepath.Dir(pdfFilePath), ".graph-*.pdf")
	if err != nil {
		return fmt.Errorf("creating temporary PDF: %w", err)
	}
	defer os.Remove(output.Name())
	if err := output.Close(); err != nil {
		return fmt.Errorf("closing temporary PDF: %w", err)
	}
	cmd := exec.Command(
		"neato",
		"-Tpdf",
		"-Goverlap=false",
		"-Gdpi=600",
		dotInputPath, "-o", output.Name())
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate PDF: %w\nOutput: %s", err, cmdOutput)
	}
	info, err := os.Stat(output.Name())
	if err != nil {
		return fmt.Errorf("reading rendered PDF: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("neato produced an empty PDF")
	}
	if err := os.Rename(output.Name(), pdfFilePath); err != nil {
		return fmt.Errorf("saving rendered PDF: %w", err)
	}

	fmt.Printf("PDF generated successfully: %s\n", pdfFilePath)
	return nil
}
