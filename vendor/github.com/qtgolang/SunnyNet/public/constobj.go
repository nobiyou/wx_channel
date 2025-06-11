// Package public /*
/*

									 Package public
------------------------------------------------------------------------------------------------
                                   程序所用到的所有公共常量
------------------------------------------------------------------------------------------------
*/
package public

import "C"
import (
	"math/rand"
	"time"
)

const SunnyVersion = "2024-08-27"

// TCP请求相关
const (
	SunnyNetMsgTypeTCPAboutToConnect = 4 //TCP即将开始连接
	SunnyNetMsgTypeTCPConnectOK      = 0 //TCP连接成功
	SunnyNetMsgTypeTCPClientSend     = 1 //客户端发送数据
	SunnyNetMsgTypeTCPClientReceive  = 2 //客户端收到数据
	SunnyNetMsgTypeTCPClose          = 3 //连接关闭或连接失败
)

// UDP请求相关
const (
	SunnyNetUDPTypeClosed  = 1 //关闭
	SunnyNetUDPTypeSend    = 2 //客户端发送数据
	SunnyNetUDPTypeReceive = 3 //客户端收到数据
)

// WebSocket相关
const (
	WebsocketConnectionOK = 1 //Websocket连接成功
	WebsocketUserSend     = 2 //Websocket发送数据
	WebsocketServerSend   = 3 //Websocket收到数据
	WebsocketDisconnect   = 4 //Websocket断开
)

// http/s 相关
const (
	HttpSendRequest = 1 //http发送请求
	HttpResponseOK  = 2 //http接收完成
	HttpRequestFail = 3 //http请求失败
)
const (
	HttpRequestPrefix  = "http" + "://"
	HttpsRequestPrefix = "https://"

	HttpMethodGET     = "GET"
	HttpMethodPOST    = "POST"
	HttpMethodPUT     = "PUT"
	HttpMethodPATCH   = "PATCH"
	HttpMethodTRACE   = "TRACE"
	HttpMethodDELETE  = "DELETE"
	HttpMethodHEAD    = "HEAD"
	HttpMethodOPTIONS = "OPTIONS"
	HttpMethodCONNECT = "CONNECT"

	TunnelConnectionEstablished = "HTTP/1.1 200 Connection Established\r\n\r\n" // 通道连接建立
	HttpResponseStatus100       = "HTTP/1.1 100 Continue\r\n\r\n"               //HTTP POST 请求 未发送Body时,回执此消息让客户端继续发送Body
	HttpDefaultPort             = "80"                                          //HTTP请求的默认端口
	HttpsDefaultPort            = "443"                                         //HTTPS请求的默认端口

	TagTcpAgreement                              = "TCP"
	TagTcpSSLAgreement                           = "TLS-TCP"
	TagMustTCP                                   = "TCP-Must"
	MaxUploadLength                              = 10240000 //<-10M  3.95M->4096000 //POST数据最大数据长度,超过次长度请求将会转成TCP方式请求
	CertificateRequestManagerRulesSend           = 1        //指定证书使用规则,发送使用
	CertificateRequestManagerRulesSendAndReceive = 2        //指定证书使用规则,发送及解析使用
	CertificateRequestManagerRulesReceive        = 3        //指定证书使用规则,解析使用
)

// 用户浏览器访问 以下地址 可以下载证书(要访问以下地址用户必须设置代理)
const (
	CertDownloadHost1 = "sunny.io" //用户浏览器访问 http://sunny.io  可以下载证书(用户必须设置代理)
	CertDownloadHost2 = "1.2.3.4"  //用户浏览器访问 http://1.2.3.4   可以下载证书(用户必须设置代理)
	/*	除了以上地址外，还有软件运行时的IP地址
		访问 软件运行时的IP地址 + 软件运行时的端口
		例如: 127.0.0.1:8888
		这种方式下载证书用户不用设置代理
	*/
)

// 其他配置常量
const (
	Space       = " " //单个空格
	NULL        = ""  //空字符串
	Nulls       = "NULL"
	CRLF        = "\r\n"          //回车+换行
	WaitingTime = 3 * time.Second //请求底层TCP连接维持多少时间
)

var NULLPtr = uintptr(0) //空字符串指针

// s5 相关常量
const (
	Socks5Version  = uint8(5)
	Socks5AuthNone = uint8(0x00)

	// Socks5AuthGSSAPI       = uint8(0x01)

	Socks5Auth = uint8(0x02)
	// Socks5AuthUnAcceptable = uint8(0xFF)

	Socks5CmdConnect     = uint8(0x01)
	Socks5CmdBind        = uint8(0x02)
	Socks5CmdUDP         = uint8(0x03)
	Socks5typeIpv4       = uint8(0x01)
	Socks5typeDomainName = uint8(0x03)
	Socks5typeIpv6       = uint8(0x04)
)

var RandomTLSValueArray = []uint16{0x0005, 0x000a, 0x002f, 0x0035, 0x003c, 0x009c, 0x009d, 0xc007, 0xc009, 0xc00a, 0xc011, 0xc012, 0xc013, 0xc014, 0xc023, 0xc027, 0xc02f, 0xc02b, 0xc030, 0xc02c, 0xcca8, 0xcca9, 0x1301, 0x1302, 0x1303, 0x5600}
var RandomTLSValueArrayLen = len(RandomTLSValueArray)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Sunny中间件自带默认证书
const (
	RootCa = `-----BEGIN CERTIFICATE-----
MIIDwjCCAqqgAwIBAgIRAQAAAAAAAAAAAAAAAAAAAAAwDQYJKoZIhvcNAQELBQAw
ajELMAkGA1UEBhMCQ04xEDAOBgNVBAgTB0JlaUppbmcxEDAOBgNVBAcTB0JlaUpp
bmcxETAPBgNVBAoTCFN1bm55TmV0MREwDwYDVQQLEwhTdW5ueU5ldDERMA8GA1UE
AxMIU3VubnlOZXQwIBcNMjIxMTA0MDcwNTM0WhgPMjEyMjEwMTEwNzA1MzRaMGox
CzAJBgNVBAYTAkNOMRAwDgYDVQQIEwdCZWlKaW5nMRAwDgYDVQQHEwdCZWlKaW5n
MREwDwYDVQQKEwhTdW5ueU5ldDERMA8GA1UECxMIU3VubnlOZXQxETAPBgNVBAMT
CFN1bm55TmV0MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzU+hPfoE
y+4+VZVUhfb0wKF7YSr79GyxNCo8/l8i1gI3pbaxv4PF/W5xWdE3LHND6b8FVmot
pXqJcalx2FP48JdAKsmlzEZcl3MngHsKH7OPSvz8p76xvlHaFutVQjQFr8R3dX3B
m8yNy6sNcP+3IrxOEUYsMWc5/lVHTyTYkruMAvCZIYzcc5Y2YXzExENbfPxwzNQh
H/XsZlc4FGaZq6DV/0oMOXSSFOXcuJo2ULW/bOQho2jZ2zG1mf+Te3i8Psoanrrf
sMXiOjB6ZH4tKv+O9NjJJi5o64Ulh35lt4qTHwGQD6pMs3yJn/l+N7kv85amLJzi
fBSbJ1eYhjUpPQIDAQABo2EwXzAOBgNVHQ8BAf8EBAMCAoQwHQYDVR0lBBYwFAYI
KwYBBQUHAwIGCCsGAQUFBwMBMA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFKC5
TwkvGBAx7xu1CyvX5chP7zOdMA0GCSqGSIb3DQEBCwUAA4IBAQDDAl162QjUsv7H
1+pn7MT/RDcqXNqBAUEc9FF6ozkRnLxdWBMLWxI8KHKm8JoBQB+TLiokSkenfMtA
7eRX7xzCBghuLi2XjMDUlaoVVKp+HNNoPSyn+UE/lUlKoCJCFgyt5p+bp9MP+YDm
pOnNjZTktyvwRj+Bgm1USzVY3IXlV+/H9la3vRi/G5n+yl3ZQMjwh6erbqwUzd6X
8j/L3BdoOkrOHzpodiAmp7Mf105Nh77EoUsh13TJy1CJLrIJzMDO1ryhzuVyxbJA
evcsWTxTr9qR/P09XImwOFmFKNimKC8IGwP/xVxqdH9WapsX6VZV5NbRG8vnqaM5
V6TbUzep
-----END CERTIFICATE-----
`
	RootKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAzU+hPfoEy+4+VZVUhfb0wKF7YSr79GyxNCo8/l8i1gI3pbax
v4PF/W5xWdE3LHND6b8FVmotpXqJcalx2FP48JdAKsmlzEZcl3MngHsKH7OPSvz8
p76xvlHaFutVQjQFr8R3dX3Bm8yNy6sNcP+3IrxOEUYsMWc5/lVHTyTYkruMAvCZ
IYzcc5Y2YXzExENbfPxwzNQhH/XsZlc4FGaZq6DV/0oMOXSSFOXcuJo2ULW/bOQh
o2jZ2zG1mf+Te3i8PsoanrrfsMXiOjB6ZH4tKv+O9NjJJi5o64Ulh35lt4qTHwGQ
D6pMs3yJn/l+N7kv85amLJzifBSbJ1eYhjUpPQIDAQABAoIBAQC8e18Wq6GdqhE1
torLFYVqFpVTBggaQ3KG5kPqbmJnv89gZZFWtV2dJLgQ8b3KI+N0Anae94kCQrVN
UHaAV87Q6Lnyzf5Uwz+blg7sp4gKxGhHOmukf69jfndN1SwHRAT4cNAOX63PHwIJ
uPX1B/0TeXXd6+MEU7Ts5VM6uCPOx5N4OlpL+A/QoPV+Uspm5C3YcZOXjTTPAtUY
JgH2nCMbCRsVIBteJQXANlSsaJP7ZgRswYKVcolNeM/zsjoNjfQUiuJbhaM6rKIa
xxV4j0TxorZpp3ablUF6HCeWoG3wRalNxVFSLd7YTXwuRcKyp3NE9KpGc4dxDEqm
4F7TmIVNAoGBAOUxLZdfMFsUmCklUikifezYkyS1eF9wjvcFJaH07KaDRPgB6VRC
hcWM+Mn6flZKtUNG5DaykxZXsh+OO8HSK85DkMterpz83cAaTvG0QxvChUNHkWqB
5dwXSDSVqgP2tDm7CqyjC9vZY3sdCeE+dzvBlL6bGTsEFPggD7emJ2z3AoGBAOVT
XmZ1yq4a1lozr2GMehYdcj7KKD18mXOlVicyl++PvJG6Hmci3RlG2sKk3PSLlRCZ
1CDhkmrNRlVwzov8uSxZOHAO+bOSqc64oRMMcyAB3uVDNicqUL2cVWiVGvcOmTc0
SgRqSU/HASMxDtT9D5nX+y0t3SL1SYRe5iBIaNJrAoGBAN5UqYx5G7iPLthjSuN6
gTu8EGmA3MeAsj8wsAP/S35wQvxvJkDF020DRujwZZQiHtqnr4TcEFGROsrfuFpa
HoKWCqUuMSc7KYZMPx67pooMVighChCO+ENcFoBkWyxDKywBpOY5uKxJovZwAgCO
Dy5ZqIiKfpxAZnMY7wZRWVebAoGAQEbCwdMoMO6CwBuWf7ABFCvCtsiwyLMgy6I+
6JOstE/EWdAh72R9NjV+4WmWKNDqwhFrvJ+dC2Rn31DUA7adLEoBoJ8B7Awinjdv
pkgqCIGduQLCre2VXd/wrHSGb1LfLPLyABTOYZb0walhb99SPRulYj9lqQO5TGnQ
9KF3B+sCgYACexhvn8ZzN637bfjaE7qge/uJcPE3CDFXxk4iYcYj6Vyv/aEBL1fJ
DPxp+LrRqH4+9/GI5pKHZf3/MU1A2ea93QIo7SCRCL7+nCHZ6er4I+XFmpugd6wq
+6JLOUgK6K0YSrzNgheVM3HWxMqT9qgrOV0CX530ia1uWSQzamhCIg==
-----END RSA PRIVATE KEY-----
`
)
