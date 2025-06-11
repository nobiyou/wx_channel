//go:build windows
// +build windows

package NFapi

func NFapi_Api_NfUdpPostSend(id uint64, remoteAddress *SockaddrInx, buf []byte, option *NF_UDP_OPTIONS) (NF_STATUS, error) {
	return Api.NfUdpPostSend(id, remoteAddress, buf, option)
}
