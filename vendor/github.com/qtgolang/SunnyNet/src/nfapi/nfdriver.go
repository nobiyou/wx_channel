//go:build windows
// +build windows

package NFapi

import (
	"reflect"

	"unsafe"

	"golang.org/x/sys/windows"
)

// C enum and #define is 4Bytes
const (
	TCP_PACKET_BUF_SIZE int32 = 8192
	UDP_PACKET_BUF_SIZE int32 = 2 * 65536
)

type DataCode int32

const (
	TCP_CONNECTED DataCode = iota
	TCP_CLOSED
	TCP_RECEIVE
	TCP_SEND
	TCP_CAN_RECEIVE
	TCP_CAN_SEND
	TCP_REQ_SUSPEND
	TCP_REQ_RESUME
	//UDP
	UDP_CREATED
	UDP_CLOSED
	UDP_RECEIVE
	UDP_SEND
	UDP_CAN_RECEIVE
	UDP_CAN_SEND
	UDP_REQ_SUSPEND
	UDP_REQ_RESUME
	//REQ RULE
	REQ_ADD_HEAD_RULE
	REQ_ADD_TAIL_RULE
	REQ_DELETE_RULES
	//CONNECT
	TCP_CONNECT_REQUEST
	UDP_CONNECT_REQUEST
	//other
	TCP_DISABLE_USER_MODE_FILTERING
	UDP_DISABLE_USER_MODE_FILTERING

	REQ_SET_TCP_OPT
	REQ_IS_PROXY

	TCP_REINJECT
	TCP_REMOVE_CLOSED
	TCP_DEFERRED_DISCONNECT

	IP_RECEIVE
	IP_SEND
	TCP_RECEIVE_PUSH
)

type DIRECTION int32

const (
	D_IN   DIRECTION = 1 // Incoming TCP connection or UDP packet
	D_OUT  DIRECTION = 2 // Outgoing TCP connection or UDP packet
	D_BOTH DIRECTION = 3 // Any direction
)

type FILTERING_FLAG uint32

const (
	NF_ALLOW                       FILTERING_FLAG = 0    // Allow the activity without filtering transmitted packets
	NF_BLOCK                       FILTERING_FLAG = 1    // Block the activity
	NF_FILTER                      FILTERING_FLAG = 2    // Filter the transmitted packets
	NF_SUSPENDED                   FILTERING_FLAG = 4    // Suspend receives from server and sends from client
	NF_OFFLINE                     FILTERING_FLAG = 8    // Emulate establishing a TCP connection with remote server
	NF_INDICATE_CONNECT_REQUESTS   FILTERING_FLAG = 16   // Indicate outgoing connect requests to API
	NF_DISABLE_REDIRECT_PROTECTION FILTERING_FLAG = 32   // Disable blocking indicating connect requests for outgoing connections of local proxies
	NF_PEND_CONNECT_REQUEST        FILTERING_FLAG = 64   // Pend outgoing connect request to complete it later using nf_complete(TCP|UDP)ConnectRequest
	NF_FILTER_AS_IP_PACKETS        FILTERING_FLAG = 128  // Indicate the traffic as IP packets via ipSend/ipReceive
	NF_READONLY                    FILTERING_FLAG = 256  // Don't block the IP packets and indicate them to ipSend/ipReceive only for monitoring
	NF_CONTROL_FLOW                FILTERING_FLAG = 512  // Use the flow limit rules even without NF_FILTER flag
	NF_REDIRECT                    FILTERING_FLAG = 1024 // Redirect the outgoing TCP connections to address specified in redirectTo
)

// NF_RULE
type NF_RULE struct {
	Protocol            INT32
	ProcessId           UINT32
	Direction           uint8
	LocalPort           UINT16
	RemotePort          UINT16
	IpFamily            INT16
	LocalIpAddress      IpAddress
	LocalIpAddressMask  IpAddress
	RemoteIpAddress     IpAddress
	RemoteIpAddressMask IpAddress
	FilteringFlag       UINT32
}

// NF_PORT_RANGE
type NF_PORT_RANGE struct {
	ValueLow  UINT16
	ValueHigh UINT16
}

// NF_RULE_EX
type NF_RULE_EX struct {
	NF_RULE
	processName         [260]UINT16
	LocalPortRange      NF_PORT_RANGE
	RemotePortRange     NF_PORT_RANGE
	RedirectTo          SockaddrInx
	LocalProxyProcessId UINT32
}

func (n *NF_RULE_EX) GetProcessName() string {
	return windows.UTF16ToString(*(*[]uint16)(unsafe.Pointer(&n.processName[0])))
}
func (n *NF_RULE_EX) SetProcessName(s string) {
	//dec := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	var si, _ = windows.UTF16FromString(s)
	l := len(si)
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&si))
	sh.Cap = l
	sh.Len = l
	copy(n.processName[:], *(*[]UINT16)(unsafe.Pointer(&sh)))

}

/**
*	UDP TDI_CONNECT request properties UNALIGNED
**/
type NF_UDP_CONN_REQUEST struct {
	FilteringFlag UINT32
	ProcessId     UINT32
	IpFamily      UINT16
	LocalAddress  SockaddrInx
	RemoteAddress SockaddrInx
}

func (op NF_UDP_OPTIONS) Clone() *NF_UDP_OPTIONS {
	var as NF_UDP_OPTIONS
	as.OptionsLength.Set(op.OptionsLength.Get())
	as.Flags.Set(op.Flags.Get())
	for i := 0; i < len(op.Options); i++ {
		as.Options[i] = op.Options[i] + 0
	}
	return &as
}
func (op NF_UDP_OPTIONS) GetBytes() (data []byte) {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&data))
	l := 4 + 4 + op.OptionsLength.Get()
	sh.Data = uintptr(unsafe.Pointer(&op))
	sh.Len = int(l)
	sh.Cap = int(l)
	return
}

// IP
type NF_IP_FLAG uint32

const (
	NFIF_NONE NF_IP_FLAG = iota
	NFIF_READONLY
)

/**
*	IP options
**/
type NF_IP_PACKET_OPTIONS struct {
	IpFamily          UINT16
	IpHeaderSize      UINT32
	CompartmentId     UINT32
	InterfaceIndex    UINT32
	SubInterfaceIndex UINT32
	Flags             UINT32
}

type NF_DATA struct {
	Code       INT32
	ID         UINT64
	BufferSize UINT32
	Buffer     byte
}

type NF_BUFFERS struct {
	InBuf, InBufLen, OutBuf, OutBufLen uint64
}
type NF_READ_RESULT struct {
	Length uint64
}
type NF_FLOWCTL_DATA struct {
	InLimit, OutLimit UINT64
}
type NF_FLOWCTL_MODIFY_DATA struct {
	FcHandle uint32
	Data     NF_FLOWCTL_DATA
}
type NF_FLOWCTL_STAT struct {
	InBytes, OutBytes UINT64
}
type NF_FLOWCTL_SET_DATA struct {
	EndpointId UINT64
	FcHandle   UINT32
}

type NF_BINDING_RULE struct {
	Protocol           INT32
	ProcessId          UINT32
	ProcessName        [260]UINT16
	LocalPort          UINT16
	IpFamily           UINT16
	LocalIpAddress     IpAddress
	LocalIpAddressMask IpAddress
	NewLocalIpAddress  IpAddress
	NewLocalPort       UINT16
	FilteringFlag      UINT32
}
