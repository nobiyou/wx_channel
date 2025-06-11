// Package public /*
/*

									 Package public
------------------------------------------------------------------------------------------------
                                   程序所用到的所有公共方法
------------------------------------------------------------------------------------------------
*/
package public

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/qtgolang/SunnyNet/src/GoWinHttp"
	"io"
	"io/ioutil"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var Timeout = "timeout"
var MaxBig = new(big.Int).Lsh(big.NewInt(1), 128)

var Theology = int64(0) //中间件唯一ID
var ProvideForwardingServiceOnly = errors.New("由于数据太长：仅提供转发服务")

// GetHost HTTP文本请求体中获取Host
func GetHost(s string) string {
	if len(s) < 11 {
		return NULL
	}
	QUrl := NULL //Url请求中的HOST
	if QUrl == NULL {
		QUrl = SubString(s, "://", "/")
	}
	if QUrl == NULL {
		QUrl = SubString(s, "://", "?")
	}
	if QUrl == NULL {
		QUrl = SubString(strings.ToLower(s), "connect ", " ")
	}
	if QUrl == NULL {
		QUrl = SubString(s+"/", "://", "/")
	}
	if QUrl == NULL {
		QUrl = SubString(s+"/", "://", "?")
	}
	HHost := NULL //协议头中的HOST
	//有些API接口 区分HOST的大小写
	//找到协议头host的位置
	index := strings.Index(strings.ToLower(s), "\nhost:")
	if index != -1 {
		sml := len(s)
		//找到 换行的位置
		if sml > index+6 {
			out := strings.Index(strings.ToLower(CopyString(s[index:])), CRLF)
			if out != -1 {
				if sml >= out+index {
					if index+6 <= out+index {
						//取出Host 是大写就是大写。小写就是小写
						HHost = strings.TrimSpace(CopyString(s[index+6 : out+index]))
					}
				}
			}
		}
	}
	if HHost == NULL {
		HHost = SubString(s, "://", "/")
	}
	if HHost == QUrl || QUrl == "" {
		//如果协议头中的HOST等于URL中的HOST 或者URL中没有HOST 直接返回协议头中的HOST
		return HHost
	}
	if HHost == "" {
		return QUrl
	}
	//将URL中的HOST分割一下 区分出HOST 和端口
	//例如URL中HOST为 1.2.3.4:8000 协议头中HOST为1.2.3.4没有端口
	ar := strings.Split(QUrl, ":")
	if len(ar) == 2 {
		//如果URL的HOST去除端口和协议头中的HOST相等那么返回URL中的HOST
		//因为协议头中的HOST不带端口号，所以按照协议头中的HOST为准
		if ar[0] == HHost {
			return QUrl
		}
	}
	//如果URL中的HOST和协议头中的HOST不一致以协议头中为准
	return HHost
}

// RemoveFile 删除文件
func RemoveFile(Filename string) error {
	return os.Remove(Filename)
}

// CheckFileIsExist 检查文件是否存在
func CheckFileIsExist(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

// WriteBytesToFile 写入数据到文件
func WriteBytesToFile(bytes []byte, Filename string) error {
	var f *os.File
	var err error
	//文件是否存在
	if CheckFileIsExist(Filename) {
		//存在 删除
		err = RemoveFile(Filename)
		if err != nil {
			return err
		}
	}
	//创建文件
	f, err = os.Create(Filename)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	// 写入
	_, err = f.Write(bytes)
	if err != nil {
		return err
	}
	return nil
}

// GetMethod 从指定数据中获取HTTP请求 Method
func GetMethod(s []byte) string {
	if len(s) < 11 {
		return NULL
	}
	method := strings.ToUpper(SubString("+"+CopyString(string(s[0:11])), "+", Space))
	if !IsHttpMethod(method) {
		return NULL
	}
	return method
}

// IsHttpMethod 通过指定字符串判断是否HTTP数据
func IsHttpMethod(methods string) bool {
	method := strings.ToUpper(methods)
	if method == HttpMethodGET {
		return true
	}
	if method == HttpMethodPUT {
		return true
	}
	if method == HttpMethodPOST {
		return true
	}
	if method == HttpMethodDELETE {
		return true
	}
	if method == HttpMethodHEAD {
		return true
	}
	if method == HttpMethodOPTIONS {
		return true
	}
	if method == HttpMethodCONNECT {
		return true
	}
	if method == HttpMethodTRACE {
		return true
	}
	if method == HttpMethodPATCH {
		return true
	}
	return false
}

// LocalBuildBody 本地文件响应数据转为Bytes
func LocalBuildBody(ContentType string, Body interface{}) []byte {
	var buffer bytes.Buffer
	var b []byte
	switch v := Body.(type) {
	case []byte:
		b = v
		break
	case string:
		b = []byte(v)
		break
	default:
		break
	}
	l := strconv.Itoa(len(b))
	buffer.WriteString("HTTP/1.1 200 OK\r\nCache-Control: no-cache, must-revalidate\r\nPragma: no-cache\r\nContent-Length: " + l + "\r\nContent-Type: " + ContentType + CRLF + CRLF)
	buffer.Write(b)
	return CopyBytes(buffer.Bytes())
}

// NewReadWriteObject 构建读写对象
func NewReadWriteObject(c net.Conn) *ReadWriteObject {
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	return &ReadWriteObject{bufio.NewReadWriter(r, w)}
}

// SplitHostPort 分割Host 和 Port 忽略IPV6地址
func SplitHostPort(ip string) (host, port string, err error) {
	arr := strings.Split(ip, ":")
	if len(arr) < 3 {
		return net.SplitHostPort(ip)
	}
	return net.SplitHostPort(ip)
	return NULL, NULL, errors.New(" 不支持 IPv6 ")
}

// IsIPv6 是否IPV6
func IsIPv6(str string) bool {
	ip := net.ParseIP(str)
	return ip.To4() == nil
}

// IsIPv4 是否IPV4
func IsIPv4(str string) bool {
	ip := net.ParseIP(str)
	return ip.To4() != nil
}

// ReadWriterPeek 在读写对象中读取n个字节,而不推进读取器
func ReadWriterPeek(f *ReadWriteObject, n int) string {
	r, _ := f.Peek(n)
	return strings.ToUpper(string(r))
}

// IsHTTPRequest 在读写对象中判断是否为HTTP请求
func IsHTTPRequest(i byte, f *ReadWriteObject) bool {
	switch i {
	case 'C':
		r := ReadWriterPeek(f, 8)
		return r == "CONNECT "
	case 'O':
		r := ReadWriterPeek(f, 8)
		return r == "OPTIONS "
	case 'H':
		r := ReadWriterPeek(f, 5)
		return r == "HEAD "
	case 'D':
		r := ReadWriterPeek(f, 7)
		return r == "DELETE "
	case 'G':
		r := ReadWriterPeek(f, 4)
		return r == "GET "
	case 'P':
		r := ReadWriterPeek(f, 5)
		if r == "POST " {
			return true
		}
		r = ReadWriterPeek(f, 4)
		if r == "PUT " {
			return true
		}
		r = ReadWriterPeek(f, 6)
		return r == "PATCH "
	case 'T':
		r := ReadWriterPeek(f, 6)
		if r == "TRACE " {
			return true
		}

	}
	return false
}

// SubString 截取字符串中间部分
func SubString(str, left, Right string) string {
	s := strings.Index(str, left)
	if s < 0 {
		return NULL
	}
	s += len(left)
	e := strings.Index(str[s:], Right)
	if e+s <= s {
		return NULL
	}
	bs := make([]byte, e)
	copy(bs, str[s:s+e])
	return string(bs)
}

// StructureBody HTTP响应体转为字节数组
func StructureBody(heads *http.Response) []byte {
	var buffer bytes.Buffer
	status := heads.StatusCode
	if status == 0 {
		status = 200
	}
	buffer.Write([]byte("HTTP/1.1 " + strconv.Itoa(status) + " " + http.StatusText(status) + CRLF))
	if heads != nil {
		if heads.Header != nil {
			for name, values := range heads.Header {
				for _, value := range values {
					buffer.Write([]byte(name + ": " + value + CRLF))
				}
			}
		}
	}

	buffer.Write([]byte(CRLF))
	if heads != nil {
		if heads.Body != nil {
			bodyBytes, _ := ioutil.ReadAll(heads.Body)
			heads.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
			buffer.Write(bodyBytes)
		}
	}
	return CopyBytes(buffer.Bytes())
}

// CStringToBytes C字符串转字节数组
func CStringToBytes(r uintptr, dataLen int) []byte {
	data := make([]byte, 0)
	if r == 0 || dataLen == 0 {
		return data
	}
	for i := 0; i < dataLen; i++ {
		data = append(data, *(*byte)(unsafe.Pointer(r + uintptr(i))))
	}
	return data
}

// WriteErr 将错误信息写入指针 请确保指针内的空间足够
func WriteErr(err error, Ptr uintptr) {
	if err == nil || Ptr == 0 {
		return
	}
	bin := []byte(err.Error())
	for i := 0; i < len(bin); i++ {
		*(*byte)(unsafe.Pointer(Ptr + uintptr(i))) = bin[i]
	}
	*(*byte)(unsafe.Pointer(Ptr + uintptr(len(bin)))) = 0
}

// IntToBytes int转字节数组
func IntToBytes(n int) []byte {
	data := int64(n)
	byteBuf := bytes.NewBuffer([]byte{})
	_ = binary.Write(byteBuf, binary.BigEndian, data)
	return byteBuf.Bytes()
}

// Int64ToBytes int64转字节数组
func Int64ToBytes(data int64) []byte {
	byteBuf := bytes.NewBuffer([]byte{})
	_ = binary.Write(byteBuf, binary.BigEndian, &data)
	ss := byteBuf.Bytes()
	return ss
}

func Float64ToBytes(float float64) []byte {
	bits := math.Float64bits(float)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, bits)
	return b
}

// BytesCombine 连接多个字节数组
func BytesCombine(pBytes ...[]byte) []byte {
	Len := len(pBytes)
	s := make([][]byte, Len)
	for index := 0; index < Len; index++ {
		s[index] = pBytes[index]
	}
	sep := []byte(NULL)
	return bytes.Join(s, sep)
}

// ContentTypeIsText 是否是文档类型
func ContentTypeIsText(ContentType string) bool {
	contentType := strings.ToLower(ContentType)
	if strings.Index(contentType, "application/json") != -1 {
		return true
	}
	if strings.Index(contentType, "application/javascript") != -1 {
		return true
	}
	if strings.Index(contentType, "application/x-javascript") != -1 {
		return true
	}
	if strings.Index(contentType, "text/") != -1 {
		return true
	}
	return false

}

// IsForward 是否是大文件类型
func IsForward(ContentType string) bool {
	contentType := strings.ToLower(ContentType)
	if strings.Index(contentType, "video/") != -1 {
		return true
	}
	if strings.Index(contentType, "audio/") != -1 {
		return true
	}
	return false
}

// ResponseToHeader HTTP响应体到字节数组
func ResponseToHeader(response *http.Response) []byte {
	if response == nil {
		return []byte{}
	}
	text := response.Status
	if text == NULL {
		text = "status code " + strconv.Itoa(response.StatusCode)
	} else {
		text = strings.TrimPrefix(text, strconv.Itoa(response.StatusCode)+Space)
	}
	if text == strconv.Itoa(response.StatusCode) || text == strconv.Itoa(response.StatusCode)+Space {
		text = ""
	}
	text = fmt.Sprintf("HTTP/%d.%d %03d %s\r\n", response.ProtoMajor, response.ProtoMinor, response.StatusCode, text)
	var bs bytes.Buffer
	bs.WriteString(text)
	for name, values := range response.Header {
		for _, value := range values {
			if name != "Transfer-Encoding" {
				bs.WriteString(name + ": " + value + CRLF)
			}
		}
	}
	bs.WriteString(CRLF)
	return CopyBytes(bs.Bytes())
}

var CertDownloadHostArray []string
var HostArray sync.Mutex

func init() {
	go func() {
		for {
			HostArray.Lock()
			CertDownloadHostArray = make([]string, 0)
			CertDownloadHostArray = append(CertDownloadHostArray, CertDownloadHost1)
			CertDownloadHostArray = append(CertDownloadHostArray, CertDownloadHost2)
			adders, err := net.InterfaceAddrs()
			if err == nil {
				for _, addr := range adders {
					if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
						if ipNet.IP.To4() != nil {
							CertDownloadHostArray = append(CertDownloadHostArray, ipNet.IP.String())
						} else if ipNet.IP.To16() != nil {
							//CertDownloadHostArray = append(CertDownloadHostArray, ipNet.IP.String())
						}
					}
				}
			}
			HostArray.Unlock()
			time.Sleep(5 * time.Second)
		}
	}()
}

// IsCerRequest 是否下载证书的请求
func IsCerRequest(request *http.Request, port int) bool {
	portStr := strconv.Itoa(port)
	if request == nil {
		return false
	}
	HostArray.Lock()
	defer HostArray.Unlock()
	if request.URL != nil {
		_host := request.URL.Host
		for _, host := range CertDownloadHostArray {
			if _host == host {
				return true
			}
			if _host == host+":"+portStr {
				return true
			}
		}
		if request.URL.Port() == portStr {
			if request.Header.Get("Sunny-Net") == "True" {
				return true
			}
			request.Header.Set("Sunny-Net", "True")
		}
	}
	return false
}

// LegitimateRequestBAK 解析HTTP 请求体,               2024-01-20之前版本的代码
func LegitimateRequestBAK(s []byte) (bool, bool, int, int, bool) {
	a := strings.ToLower(CopyString(string(s)))
	arrays := strings.Split(a, Space)
	isHttpRequest := false
	Method := GetMethod(s)
	if IsHttpMethod(Method) && Method != HttpMethodCONNECT {
		if SubString(string(s), Space, Space) != "" {
			isHttpRequest = true
		}
	}
	if len(arrays) > 1 {
		//Body中是否有长度
		islet := strings.Index(a, "content-length: ") != -1
		if islet {
			ContentLength, _ := strconv.Atoi(SubString(a, "content-length: ", CRLF))
			if ContentLength == 0 {
				// 有长度  但长度为0 直接验证成功
				return islet, true, 0, ContentLength, isHttpRequest
			}
			arr := bytes.Split(s, []byte(CRLF+CRLF))
			if len(arr) < 2 {
				// 读取验证失败
				return islet, false, 0, ContentLength, isHttpRequest
			}
			var b bytes.Buffer
			for i := 0; i < len(arr); i++ {
				if i != 0 {
					b.Write(CopyBytes(arr[i]))
					b.Write([]byte{13, 10, 13, 10})
				}
			}
			if b.Len() == ContentLength || b.Len()-4 == ContentLength {
				b.Reset()
				// 有长度  读取验证成功
				return islet, true, 0, ContentLength, isHttpRequest
			}
			v := b.Len() - 4
			b.Reset()
			return islet, false, v, ContentLength, isHttpRequest
		} else if strings.Index(a, "transfer-encoding: chunked") != -1 {
			islet = true
			arr := bytes.Split(s, []byte(CRLF+CRLF))
			if len(arr) < 2 {
				// 读取验证失败
				return islet, false, 0, 0, isHttpRequest
			}
			arrays = strings.Split(string(arr[1]), CRLF)
			if len(arr) < 1 {
				return islet, false, 0, 0, isHttpRequest
			}
			ContentLength2, _ := strconv.ParseInt(arrays[0], 16, 64)
			ContentLength := int(ContentLength2) + len(arrays[0]) + 2
			var b bytes.Buffer
			for i := 0; i < len(arr); i++ {
				if i > 0 {
					b.Write(CopyBytes(arr[i]))
					b.Write([]byte{13, 10, 13, 10})
				}
			}
			if b.Len() == ContentLength || b.Len()-4 == ContentLength {
				b.Reset()
				// 有长度  读取验证成功
				return islet, true, 0, ContentLength, isHttpRequest
			}
			v := b.Len() - 4
			b.Reset()
			return islet, false, v, ContentLength, isHttpRequest
		}
		Method := GetMethod(s)
		if (Method == HttpMethodGET || Method == HttpMethodOPTIONS || Method == HttpMethodHEAD) && len(s) > 4 && CopyString(string(s[len(s)-4:])) == CRLF+CRLF {
			return false, true, 0, 0, isHttpRequest
		}
		//没有长度  读取验证失败
		return false, false, 0, 0, isHttpRequest
	}
	return false, false, 0, 0, isHttpRequest

}

// LegitimateRequest 解析HTTP 请求体，                 2024-01-20修改后的代码
func LegitimateRequest(s []byte) (bool, bool, int, int, bool) {
	a := strings.ToLower(CopyString(string(s)))
	arrays := strings.Split(a, Space)
	isHttpRequest := false
	Method := GetMethod(s)
	if IsHttpMethod(Method) && Method != HttpMethodCONNECT {
		if SubString(string(s), Space, Space) != "" {
			isHttpRequest = true
		}
	}
	if len(arrays) > 1 {
		//Body中是否有长度
		islet := strings.Index(a, "content-length: ") != -1
		if islet {
			ContentLength, _ := strconv.Atoi(SubString(a+CRLF, "content-length: ", CRLF))
			if ContentLength == 0 {
				if strings.Contains(a, CRLF+CRLF) {
					// 有长度  但长度为0 直接验证成功,并且有CRLF+CRLF
					return islet, true, 0, ContentLength, isHttpRequest
				}
				// 有长度  但长度为0 但是没有 有CRLF+CRLF 直接验证失败
				return islet, false, 0, ContentLength, isHttpRequest
			}
			arr := bytes.Split(s, []byte(CRLF+CRLF))
			if len(arr) < 2 {
				// 读取验证失败
				return islet, false, 0, ContentLength, isHttpRequest
			}
			var b bytes.Buffer
			for i := 0; i < len(arr); i++ {
				if i != 0 {
					b.Write(CopyBytes(arr[i]))
					b.Write([]byte{13, 10, 13, 10})
				}
			}
			if b.Len() == ContentLength || b.Len()-4 == ContentLength {
				b.Reset()
				// 有长度  读取验证成功
				return islet, true, 0, ContentLength, isHttpRequest
			}
			v := b.Len() - 4
			b.Reset()
			return islet, false, v, ContentLength, isHttpRequest
		} else if strings.Index(a, "transfer-encoding: chunked") != -1 {
			islet = true
			arr := bytes.Split(s, []byte(CRLF+CRLF))
			if len(arr) < 2 {
				// 读取验证失败
				return islet, false, 0, 0, isHttpRequest
			}
			BodyLen := 0
			var bs bytes.Buffer
			for i := 0; i < len(arr); i++ {
				if i != 0 {
					if len(arr[i]) == 0 {
						continue
					}
					bs.Write(CopyBytes(arr[i]))
					bs.Write([]byte{13, 10, 13, 10})
					BodyLen += len(arr[i]) + 4
				}
			}
			reader := bufio.NewReader(bytes.NewReader(bs.Bytes()))
			bLen := int64(0)
			for {
				T, _, e := reader.ReadLine()
				ContentLength2, _ := strconv.ParseInt(string(T), 16, 64)
				if ContentLength2 == 0 && e != nil {
					break
				}
				bLen += ContentLength2 + int64(len(T)+2)
				if ContentLength2 == 0 {
					bLen += 2
					break
				}
				p, _ := reader.Discard(int(ContentLength2))
				if p != int(ContentLength2) {
					v := bLen - ContentLength2 + int64(p)
					return islet, false, int(v), 0, isHttpRequest
				}
				bLen += 2
				p, _ = reader.Discard(2)
				if p != 2 {
					v := bLen - 2 + int64(p)
					return islet, false, int(v), 0, isHttpRequest
				}
			}
			if int64(BodyLen) == bLen {
				return islet, true, BodyLen, BodyLen, isHttpRequest
			}
			return islet, false, int(bLen), int(bLen), isHttpRequest
		}
		if (Method == HttpMethodGET || Method == HttpMethodOPTIONS || Method == HttpMethodHEAD) && len(s) > 4 && CopyString(string(s[len(s)-4:])) == CRLF+CRLF {
			return false, true, 0, 0, isHttpRequest
		}
		//没有长度  读取验证失败
		return false, false, 0, 0, isHttpRequest
	}
	return false, false, 0, 0, isHttpRequest

}

// BuildRequest 处理解析HTTP请求结构
func BuildRequest(RawData []byte, host, source, DefaultPort string, setProxyHost func(s string), br *ReadWriteObject) (reqs *http.Request, length int) {
	defer func() {
		if reqs != nil {
			if reqs.URL != nil {
				h := reqs.URL.Host
				arr := strings.Split(h, ":")
				if len(arr) == 2 {
					if reqs.URL.Scheme == "http" && arr[1] == "80" {
						us := strings.ReplaceAll(reqs.URL.String(), CopyString(h), CopyString(arr[0]))
						reqs.URL, _ = url.Parse(CopyString(us))
					}
					if reqs.URL.Scheme == "https" && arr[1] == "443" {
						us := strings.ReplaceAll(reqs.URL.String(), CopyString(h), CopyString(arr[0]))
						reqs.URL, _ = url.Parse(CopyString(us))
					}
				}
				arr = nil
				h = reqs.URL.Host
				if strings.Contains(h, "127.0.0.1") && source != "" {
					s := CopyString(source)
					a1 := strings.Split(h, ":")
					if len(a1) > 1 {
						s = CopyString(source) + ":" + CopyString(a1[1])
					}
					reqs.URL.Host = s
					reqs.Host = s
					a1 = nil
				}
			}
		}
	}()
	Scheme := HttpRequestPrefix
	if DefaultPort == HttpsDefaultPort {
		Scheme = HttpsRequestPrefix
	}
	var mPort = func(mHost string) string {
		arr := strings.Split(mHost, ":")
		if len(arr) == 2 {
			if arr[1] == DefaultPort {
				return NULL
			}
			return ":" + CopyString(arr[1])
		}
		return NULL
	}(host)
	var isPort = func(sHost string) string {
		if strings.Index(sHost, ":") == -1 {
			return sHost + mPort
		}
		return sHost
	}
	Method := GetMethod(RawData)
	if !IsHttpMethod(Method) {
		return nil, 0
	}
	isHttpProxy := false
	lasts := string(RawData)
	Path := SubString(lasts, Space, Space)
	if strings.Index(Path, HttpRequestPrefix) != -1 || strings.Index(Path, HttpsRequestPrefix) != -1 {
		isHttpProxy = true
		if strings.HasPrefix(Path, HttpsRequestPrefix) {
			Path = strings.Replace(Path, HttpsRequestPrefix, NULL, 1)
		}
		if strings.HasPrefix(Path, HttpRequestPrefix) {
			Path = strings.Replace(Path, HttpRequestPrefix, NULL, 1)
		}
		if strings.Index(Path, "/") == -1 {
			Path = NULL
		} else {
			Path = strings.TrimSpace(SubString(Path+Space, "/", Space))
			if Path != NULL {
				Path = "/" + Path
			}
		}
	}
	BodyIndex := bytes.Index(RawData, []byte(CRLF+CRLF))
	buff := CopyBytes(RawData[BodyIndex+4:])
	arr := strings.Split(strings.ReplaceAll(string(CopyBytes(RawData[:BodyIndex+4])), "\r", NULL), "\n")
	BodyLength := 0
	req := &http.Request{
		Method:     Method,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       nil,
	}
	defer func() {
		if req.URL == nil {
			return
		}
		jHost := req.URL.Host
		Ars := strings.Split(req.URL.Host, ":")
		if len(Ars) == 2 {
			if Ars[1] == DefaultPort {
				jHost = Ars[0]
			}
		}
		m := req.Header["Host"]
		if len(m) > 0 {
			req.Header["Host"] = []string{jHost}
			return
		}
		m = req.Header["host"]
		if len(m) > 0 {
			req.Header["host"] = []string{jHost}
		}
	}()
	TransferEncoding := false
	var HeadArr []string
	for index := 0; index < len(arr); index++ {
		if index != 0 {
			HeadArr = strings.Split(arr[index], ":")
			if len(HeadArr) > 1 {
				Name := strings.TrimSpace(strings.ReplaceAll(CopyString(HeadArr[0]), ":", NULL))
				value := ""
				for i, v := range HeadArr[1:] {
					if i == 0 {
						value = CopyString(v)
					} else {
						value += ":" + CopyString(v)
					}
				}
				value = strings.TrimSpace(value)
				if strings.ToLower(Name) == "transfer-encoding" {
					TransferEncoding = true
					continue
				}
				if strings.ToLower(Name) == "expect" {
					//Expect: 100-continue
					continue
				}
				if req.Header[Name] != nil {
					req.Header[Name] = append(req.Header[Name], value)
				} else {
					req.Header[Name] = []string{value}
				}
				if req.URL == nil && strings.ToUpper(HeadArr[0]) == "HOST" {
					if strings.HasPrefix(Path, host) {
						Path = CopyString(Path[len(host):])
					} else if strings.HasPrefix(Path, HttpRequestPrefix+host) {
						Path = CopyString(Path[len(HttpRequestPrefix+host):])
					} else if strings.HasPrefix(Path, HttpsRequestPrefix+host) {
						Path = CopyString(Path[len(HttpsRequestPrefix+host):])
					}
					mHost := host
					if mHost == NULL {
						if isHttpProxy {
							mHost = CopyString(HeadArr[1])
						} else {
							mHost = isPort(CopyString(HeadArr[1]))
						}
					}
					if HeadArr[1] == Path {
						_u, _ := url.Parse(CopyString(strings.ReplaceAll(Scheme+mHost, Space, NULL)))
						req.URL = _u
						//req.RequestURI = strings.ReplaceAll(Scheme+mHost, Space, NULL)
					} else {
						_u, _ := url.Parse(CopyString(strings.ReplaceAll(Scheme+mHost+Path, Space, NULL)))
						req.URL = _u
						//req.RequestURI = strings.ReplaceAll(Scheme+mHost+Path, Space, NULL)
					}
					req.Host = strings.ReplaceAll(mHost, Space, NULL)
					if isHttpProxy {
						setProxyHost(host)
					}
					host = strings.ReplaceAll(mHost, Space, NULL)
				}
				if strings.ToUpper(HeadArr[0]) == "CONTENT-LENGTH" {
					bodyLen, _ := strconv.Atoi(strings.TrimSpace(CopyString(HeadArr[1])))
					req.ContentLength = int64(bodyLen)
					if bodyLen == len(buff) {
						BodyLength = len(buff)
						req.Body = ioutil.NopCloser(bytes.NewBuffer(buff))
					}
				}
			} else if len(HeadArr) == 1 {
				if HeadArr[0] != "" {
					Name := strings.TrimSpace(strings.ReplaceAll(CopyString(HeadArr[0]), ":", NULL))
					req.Header[Name] = []string{}
				}
			}
		}
	}
	if req.URL == nil {
		if strings.HasPrefix(Path, host) {
			Path = CopyString(Path[len(host):])
		} else if strings.HasPrefix(Path, HttpRequestPrefix+host) {
			Path = CopyString(Path[len(HttpRequestPrefix+host):])
		} else if strings.HasPrefix(Path, HttpsRequestPrefix+host) {
			Path = CopyString(Path[len(HttpsRequestPrefix+host):])
		}
		if host == Path {
			_u, _ := url.Parse(CopyString(strings.ReplaceAll(Scheme+host, Space, NULL)))
			req.URL = _u
		} else {
			_u, _ := url.Parse(CopyString(strings.ReplaceAll(Scheme+host+Path, Space, NULL)))
			req.URL = _u
		}
		req.Host = strings.ReplaceAll(host, Space, NULL)
	}
	if req.Body != nil || Method == HttpMethodGET || Method == HttpMethodCONNECT || Method == HttpMethodOPTIONS {
		return req, BodyLength
	}
	req.Header.Del("Transfer-Encoding")
	defer func() {
		if reqs != nil {
			if reqs.Method != HttpMethodGET && reqs.Method != HttpMethodCONNECT && reqs.Method != HttpMethodOPTIONS {
				if reqs.Header != nil {
					reqs.Header.Set("Content-Length", strconv.Itoa(length))
				}
			}
		}
	}()
	if BodyIndex < 1 {
		return req, BodyLength
	}
	if bytes.Contains(buff, []byte(CRLF)) && TransferEncoding {
		var buf bytes.Buffer
		buf.Write(CopyBytes(RawData))
		buffBody := make([]byte, 512)
		for {
			if bytes.HasSuffix(buf.Bytes(), []byte{13, 10, 48, 13, 10, 13, 10}) {
				break
			}
			l, e := br.Read(buffBody)
			if l > 0 {
				buf.Write(CopyBytes(buffBody[0:l]))
			}
			if e != nil {
				break
			}
		}
		r := bufio.NewReader(&buf)
		ret, _ := http.ReadRequest(r)
		if ret != nil {
			if ret.Body != nil {
				body, _ := io.ReadAll(ret.Body)
				buff = CopyBytes(body)
				body = nil
				_ = ret.Body.Close()
			}
		}
		buffBody = make([]byte, 0)
		buffBody = nil
		buf.Reset()
		ret = nil
		r = nil
	}
	BodyLength = len(buff)
	if req.Body != nil {
		_ = req.Body.Close()
		req.Body = nil
	}
	if len(buff) == 0 {
		return req, BodyLength
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(CopyBytes(buff)))
	if req.ContentLength == 0 {
		req.ContentLength = int64(BodyLength)
	}
	RawData = make([]byte, 0)
	RawData = nil
	return req, BodyLength
}

// CopyBytes 拷贝 字节数组避免内存泄漏
func CopyBytes(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

// CopyString 拷贝字符串 避免内存泄漏
func CopyString(src string) string {
	dst := make([]byte, len(src))
	copy(dst, src)
	return string(dst)
}

// CopyBuffer 转发数据
func CopyBuffer(dst *ReadWriteObject, src io.Reader, dstConn net.Conn, srcConn *GoWinHttp.PoolInfo, SetBodyValue func([]byte, error) []byte, ExpectLen int, SetReqHeadsValue func(string) []byte, ContentType string, setOut func(), Method string) {
	size := 512
	MaxSize := 5 * 1024 * 1024 //5M
	IsText := ContentTypeIsText(ContentType)
	if IsText && ExpectLen < 1 {
		MaxSize = 5 * 1024 * 1024 * 10 //50M
		size = 32 * 1024
	}
	buf := make([]byte, size)
	var buff bytes.Buffer
	defer func() {
		buff.Reset()
		buf = make([]byte, 0)
		buf = nil
	}()
	var isForward = false
	// 是否是大文件类型 是的话,不判断长度直接转发 并且长度大于指定值(5M) 则直接转发
	var ToIsForward = IsForward(ContentType) && (ExpectLen < 1 || ExpectLen > 5*1024*1024) //5M
	Doe := func(Bs []byte) []byte {
		return CopyBytes(Bs)
	}
	if Method == HttpMethodHEAD {
		_ = srcConn.Conn.Close()
		SetBodyValue(nil, nil)
		bb := SetReqHeadsValue(strconv.Itoa(ExpectLen))
		_ = dstConn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = dst.Write(bb)
		bb = make([]byte, 0)
		return
	}
	for {
		_ = srcConn.SetDeadline(time.Now().Add(time.Duration(30) * time.Second))
		nr, er := src.Read(buf)
		if nr > 0 {
			buff.Write(buf[0:nr])
			if ToIsForward || (isForward || ExpectLen > MaxSize || (ExpectLen < 1 && buff.Len() > MaxSize)) {
				_ = dstConn.SetDeadline(time.Now().Add(5 * time.Second))
				if isForward == false {
					isForward = true
					setOut()
					SetBodyValue([]byte{}, ProvideForwardingServiceOnly)
					_, _ = dst.Write(SetReqHeadsValue("-1"))

				}
				nr = buff.Len()
				nw, ew := dst.Write(CopyBytes(buff.Bytes()))
				buff.Reset()
				if nw < 0 || nr < nw {
					nw = 0
					if ew == nil {
						ew = errors.New("invalid write result")
					}
				}
				if ew != nil {
					return
				}
				if nr != nw {
					return
				}
				if er != nil {
					return
				}
				continue
			} else if ExpectLen > 0 && ExpectLen == buff.Len() {
				er = io.EOF
			}
		}
		if er != nil {
			if buff.Len() >= 0 {

				_body := Doe(SetBodyValue(buff.Bytes(), nil))
				_head := SetReqHeadsValue(strconv.Itoa(len(_body)))
				_ = dstConn.SetDeadline(time.Now().Add(5 * time.Second))
				_, _ = dst.Write(_head)
				_, _ = dst.Write(_body)
				_head = make([]byte, 0)
				_body = make([]byte, 0)
			}
			return
		}
	}
}
