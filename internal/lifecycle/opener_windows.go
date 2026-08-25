//go:build windows

package lifecycle

import (
	"context"
	"fmt"
	"os/exec"
)

const channelProtocolURL = "weixin://dl/channels"

type protocolPageOpener struct{}

func newPageOpener() PageOpener {
	return protocolPageOpener{}
}

func (protocolPageOpener) Open(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", channelProtocolURL)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open video channel protocol: %w", err)
	}
	return nil
}
