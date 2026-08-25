//go:build !windows

package lifecycle

import "context"

type unsupportedProcessProbe struct{}

func newProcessProbe() ProcessProbe {
	return unsupportedProcessProbe{}
}

func (unsupportedProcessProbe) Running(context.Context) (bool, error) {
	return false, nil
}
