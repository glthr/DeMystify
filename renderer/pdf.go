package pdf

import (
	"fmt"
	"os/exec"
)

const pdfFilePath = "generated/graph.pdf"

// RenderPDF generates a PDF visualization using Neato
// NOTE: it would be great to replace this system call with a function call (library)
func RenderPDF(dotFilePath string) error {
	cmd := exec.Command(
		"neato",
		"-Tpdf",
		"-Goverlap=false",
		"-Gdpi=600",
		dotFilePath, "-o", pdfFilePath)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate PDF: %w\nOutput: %s", err, cmdOutput)
	}

	fmt.Printf("PDF generated successfully: %s\n", pdfFilePath)
	return nil
}
