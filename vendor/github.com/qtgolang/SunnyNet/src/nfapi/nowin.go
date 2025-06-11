//go:build !windows
// +build !windows

package NFapi

func NFapi_Api_NfUdpPostSend(id uint64, remoteAddress any, buf []byte, option any) (int32, error) {
	return 0, nil
}
