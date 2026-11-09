package layout

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	maxJSONBytes   = 64 << 20
	maxDOTBytes    = 16 << 20
	maxErrorBytes  = 64 << 10
	DefaultTimeout = 5 * time.Minute
)

// Run uses the same Neato layout flags as PDF rendering. The JSON output uses
// points regardless of the raster DPI, so the browser must not multiply by600.
func Run(dot string) (Drawing, error) {
	return RunContext(context.Background(), dot)
}

// RunContext bounds execution by both the caller and DefaultTimeout. It passes
// DOT through stdin and never invokes a shell or treats a filename as an option.
func RunContext(ctx context.Context, dot string) (Drawing, error) {
	if ctx == nil {
		return Drawing{}, fmt.Errorf("layout context is nil")
	}
	if len(dot) > maxDOTBytes {
		return Drawing{}, fmt.Errorf("DOT exceeds layout input size limit")
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "neato", "-Tjson", "-Goverlap=false", "-Gdpi=600")
	cmd.Stdin = strings.NewReader(dot)
	output := cappedOutput{maximum: maxJSONBytes}
	diagnostic := cappedOutput{maximum: maxErrorBytes, discard: true}
	cmd.Stdout, cmd.Stderr = &output, &diagnostic
	err := cmd.Run()
	if ctx.Err() != nil {
		return Drawing{}, fmt.Errorf("Neato layout canceled or timed out: %w", ctx.Err())
	}
	if err != nil {
		return Drawing{}, fmt.Errorf("Neato layout failed: %w\n%s", err, diagnostic.buffer.String())
	}
	drawing, err := FromJSON(output.buffer.Bytes())
	if err != nil {
		return Drawing{}, fmt.Errorf("Neato produced invalid layout: %w", err)
	}
	return drawing, nil
}

type cappedOutput struct {
	buffer  bytes.Buffer
	maximum int
	discard bool
}

func (output *cappedOutput) Write(data []byte) (int, error) {
	remaining := output.maximum - output.buffer.Len()
	if len(data) <= remaining {
		return output.buffer.Write(data)
	}
	if remaining > 0 {
		output.buffer.Write(data[:remaining])
	}
	if output.discard {
		return len(data), nil
	}
	return remaining, fmt.Errorf("Neato layout output exceeds size limit")
}
