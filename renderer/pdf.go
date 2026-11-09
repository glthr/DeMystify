package pdf

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const pdfFilePath = "generated/graph.pdf"

// RenderPDF generates a PDF visualization using Neato
// NOTE: it would be great to replace this system call with a function call (library)
func RenderPDF(dotFilePath string) error {
	_, err := renderPDF(dotFilePath, false, false)
	return err
}

// RenderPDFAndLayout emits PDF and Graphviz drawing JSON from one layout pass.
// The browser can replay the same positions, splines and arrows as the PDF.
func RenderPDFAndLayout(dotFilePath string) ([]byte, error) {
	return renderPDF(dotFilePath, true, false)
}

// RenderPinnedPDF preserves the prepared node arrangement while routing edges.
func RenderPinnedPDF(dotFilePath string) error {
	_, err := renderPDF(dotFilePath, false, true)
	return err
}

// RenderPinnedPDFAndLayout shares one pinned layout pass between PDF and HTML.
func RenderPinnedPDFAndLayout(dotFilePath string) ([]byte, error) {
	return renderPDF(dotFilePath, true, true)
}

func renderPDF(dotFilePath string, withLayout, pinned bool) ([]byte, error) {
	// Graphviz does not accept "--"; absolute paths keep leading dashes literal.
	dotInputPath, err := filepath.Abs(dotFilePath)
	if err != nil {
		return nil, fmt.Errorf("resolving DOT input path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pdfFilePath), 0755); err != nil {
		return nil, fmt.Errorf("creating PDF output directory: %w", err)
	}
	output, err := os.CreateTemp(filepath.Dir(pdfFilePath), ".graph-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("creating temporary PDF: %w", err)
	}
	defer os.Remove(output.Name())
	if err := output.Close(); err != nil {
		return nil, fmt.Errorf("closing temporary PDF: %w", err)
	}
	args := []string{
		"-Tpdf",
		"-Goverlap=false",
		"-Gdpi=600",
		dotInputPath, "-o", output.Name()}
	if pinned {
		args = append([]string{"-n2"}, args...)
	}
	var layoutPath string
	if withLayout {
		layout, err := os.CreateTemp(filepath.Dir(pdfFilePath), ".graph-*.json")
		if err != nil {
			return nil, fmt.Errorf("creating temporary layout: %w", err)
		}
		layoutPath = layout.Name()
		defer os.Remove(layoutPath)
		if err := layout.Close(); err != nil {
			return nil, fmt.Errorf("closing temporary layout: %w", err)
		}
		args = append(args, "-Tjson", "-o", layoutPath)
	}
	cmd := exec.Command("neato", args...)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w\nOutput: %s", err, cmdOutput)
	}
	info, err := os.Stat(output.Name())
	if err != nil {
		return nil, fmt.Errorf("reading rendered PDF: %w", err)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("neato produced an empty PDF")
	}
	var layoutData []byte
	if withLayout {
		layoutData, err = os.ReadFile(layoutPath)
		if err != nil {
			return nil, fmt.Errorf("reading rendered layout: %w", err)
		}
		if !json.Valid(layoutData) {
			return nil, fmt.Errorf("neato produced invalid drawing JSON")
		}
	}
	if err := os.Rename(output.Name(), pdfFilePath); err != nil {
		return nil, fmt.Errorf("saving rendered PDF: %w", err)
	}

	fmt.Printf("PDF generated successfully: %s\n", pdfFilePath)
	return layoutData, nil
}
