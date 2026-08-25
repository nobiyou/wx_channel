//go:build windows

package lifecycle

import (
	"context"
	"fmt"
	"os/exec"
)

type tasklistProcessProbe struct{}

func newProcessProbe() ProcessProbe {
	return tasklistProcessProbe{}
}

func (tasklistProcessProbe) Running(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "tasklist.exe", "/FI", "IMAGENAME eq "+wechatProcessName, "/NH", "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("tasklist %s: %w", wechatProcessName, err)
	}
	return parseTasklistOutput(output)
}
