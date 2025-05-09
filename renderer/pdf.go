package pdf

import (
	"fmt"
	"log"
	"os/exec"
)

const pdfFilePath = "generated/graph.pdf"

// RenderPDF generates a PDF visualization using Neato
// NOTE: it would be great to replace this system call with a function call (library)
func RenderPDF(dotFilePath string) {
	cmd := exec.Command(
		"neato",
		"-Tpdf",
		"-Goverlap=false",
		"-Gdpi=600",
		dotFilePath, "-o", pdfFilePath)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to generate PDF: %v\nOutput: %s\n", err, cmdOutput)
		return
	}

	fmt.Printf("PDF generated successfully: %s\n", pdfFilePath)
}
