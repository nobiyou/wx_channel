//go:build !windows

package lifecycle

import (
	"context"
	"errors"
)

type unsupportedPageOpener struct{}

func newPageOpener() PageOpener {
	return unsupportedPageOpener{}
}

func (unsupportedPageOpener) Open(context.Context) error {
	return errors.New("video channel page opener is only available on Windows")
}
