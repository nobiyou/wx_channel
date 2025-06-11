package Certificate

import "C"
import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/crypto/pkcs"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"
)

type CertManager struct {
	Tls          *tls.Config
	PrivateKey   string
	Certificates string
	Cert         string
}

var Lock sync.Mutex
var Map = make(map[int]*CertManager)

func CreateCertificate() int {
	Lock.Lock()
	defer Lock.Unlock()
	w := &CertManager{Tls: &tls.Config{}}
	Context := newMessageId()
	Map[Context] = w
	return Context
}

// RemoveCertificate 释放 证书管理器 对象
func RemoveCertificate(Context int) {
	Lock.Lock()
	defer Lock.Unlock()
	c := LoadCertificateContext(Context)
	if c == nil {
		return
	}
	c = nil
	delete(Map, Context)
}

func LoadCertificateContext(Context int) *CertManager {
	s := Map[Context]
	if s == nil {
		return nil
	}
	return s
}

func (c *CertManager) setCert(obj any, obj2 ...any) bool {
	isCert := func(o []byte) bool {
		block, _ := pem.Decode(o)
		if block != nil {
			private, e := x509.ParsePKCS1PrivateKey(block.Bytes)
			if e != nil {
				privateKey1, er := x509.ParsePKCS8PrivateKey(block.Bytes)
				if er != nil {
					return false
				}
				private, _ = privateKey1.(*rsa.PrivateKey)
			}
			if private == nil {
				return false
			}
			return true
		}
		return false
	}
	switch v := obj.(type) {
	case string:
		if isCert([]byte(v)) {
			c.Cert = v
			return true
		}
		return false
	case []byte:
		if len(obj2) >= 1 {
			Block1, E1 := pem.Decode(v)
			m2 := obj2[0].([]byte)
			if m2 == nil {
				return false
			}
			Block2, E2 := pem.Decode(m2)
			if E1 != nil && E2 != nil {
				pemData := pem.EncodeToMemory(Block1)
				pemData = append(pemData, pem.EncodeToMemory(Block2)...)
				if isCert(pemData) {
					c.Cert = string(pemData)
					return true
				}
			}
			return false
		}
		if isCert(v) {
			c.Cert = string(v)
			return true
		}
		return false
	}
	return false
}

// LoadP12Certificate 证书管理器 载入p12证书
func (c *CertManager) LoadP12Certificate(Name, Password string) bool {
	if c.Tls == nil {
		return false
	}
	p, Certificates, private, pemData, e := AddP12Certificate(Name, Password)
	if e != nil {
		return false
	}
	c.PrivateKey = private
	c.Certificates = Certificates
	c.Tls.Certificates = []tls.Certificate{*p}
	c.setCert(pemData)
	return true
}

func (c *CertManager) LoadX509KeyPair(capath, keyPath string) bool {
	if c.Tls == nil {
		return false
	}
	keyPEMBlock, err := os.ReadFile(keyPath)
	if err != nil {
		return false
	}
	CaPEMBlock, err := os.ReadFile(capath)
	if err != nil {
		return false
	}
	c.PrivateKey = string(keyPEMBlock)
	c.Certificates = string(CaPEMBlock)
	a, e := tls.LoadX509KeyPair(capath, keyPath)
	if e != nil {
		return false
	}
	c.Tls.Certificates = []tls.Certificate{a}
	c.setCert(CaPEMBlock, keyPEMBlock)
	return true
}

func (c *CertManager) LoadX509Certificate(host string, ca, key string) bool {
	if c.Tls == nil {
		return false
	}
	c.PrivateKey = key
	c.Certificates = ca

	cc, err := c.loadRootCa([]byte(ca))
	if err != nil {
		return false
	}
	k, err := loadRootKey([]byte(key))
	if err != nil {
		return false
	}
	a, b, e := generatePem(host, cc, k)
	if e != nil {
		return false
	}
	cer, err := tls.X509KeyPair(a, b)
	if err != nil {
		return false
	}
	c.Tls.Certificates = []tls.Certificate{cer}
	c.setCert(a, b)
	return true
}

func (c *CertManager) SetInsecureSkipVerify(b bool) bool {
	if c.Tls == nil {
		return false
	}
	c.Tls.InsecureSkipVerify = b
	return true
}

// SetServerName 证书管理器 设置ServerName
func (c *CertManager) SetServerName(name string) bool {
	if c.Tls == nil {
		return false
	}
	c.Tls.ServerName = name
	return true
}

// GetServerName 证书管理器 取ServerName
func (c *CertManager) GetServerName() string {
	if c.Tls == nil {
		return public.NULL
	}
	return c.Tls.ServerName
}

// AddCertPoolPath 证书管理器 设置信任的证书 从 文件
func (c *CertManager) AddCertPoolPath(path string) bool {
	if c.Tls == nil {
		return false
	}
	if c.Tls == nil {
		return false
	}

	aCrt, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}
	if c.Tls.ClientCAs == nil {
		c.Tls.ClientCAs = x509.NewCertPool()
	}
	if !c.Tls.ClientCAs.AppendCertsFromPEM(aCrt) {
		cert, err := x509.ParseCertificate(aCrt)
		if err != nil {
			panic(err)
		}
		// 将证书转换为 PEM 格式
		pemBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})
		return c.Tls.ClientCAs.AppendCertsFromPEM(pemBytes)
	}
	c.setCert(aCrt)
	return true
}

// AddCertPoolText 证书管理器 设置信任的证书 从 文本
func (c *CertManager) AddCertPoolText(cer string) bool {
	if c.Tls == nil {
		return false
	}
	if c.Tls == nil {
		return false
	}
	if c.Tls.ClientCAs == nil {
		c.Tls.ClientCAs = x509.NewCertPool()
	}
	if !c.Tls.ClientCAs.AppendCertsFromPEM([]byte((cer))) {
		cert, err := x509.ParseCertificate([]byte((cer)))
		if err != nil {
			panic(err)
		}
		// 将证书转换为 PEM 格式
		pemBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})
		return c.Tls.ClientCAs.AppendCertsFromPEM(pemBytes)
	}
	c.setCert(cer)
	return true
}

// AddClientAuth 证书管理器 设置ClientAuth
func (c *CertManager) AddClientAuth(val int) bool {
	if c.Tls == nil {
		return false
	}
	switch val {
	case 0:
		c.Tls.ClientAuth = tls.NoClientCert
		break
	case 1:
		c.Tls.ClientAuth = tls.RequestClientCert
		break
	case 2:
		c.Tls.ClientAuth = tls.RequireAnyClientCert
		break
	case 3:
		c.Tls.ClientAuth = tls.VerifyClientCertIfGiven
		break
	case 4:
		c.Tls.ClientAuth = tls.RequireAndVerifyClientCert
		break
	default:
		c.Tls.ClientAuth = tls.NoClientCert
		break
	}
	return true
}

func generatePem(host string, rootCa *x509.Certificate, rootKey *rsa.PrivateKey) ([]byte, []byte, error) {
	serialNumber, _ := rand.Int(rand.Reader, public.MaxBig) //返回在 [0, max) 区间均匀随机分布的一个随机值
	template := x509.Certificate{
		SerialNumber: serialNumber, // SerialNumber 是 CA 颁布的唯一序列号，在此使用一个大随机数来代表它
		Subject: pkix.Name{ //Name代表一个X.509识别名。只包含识别名的公共属性，额外的属性被忽略。
			CommonName: host,
		},
		NotBefore:      time.Now().AddDate(-1, 0, 0),
		NotAfter:       time.Now().AddDate(1, 0, 0),
		KeyUsage:       x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, //KeyUsage 与 ExtKeyUsage 用来表明该证书是用来做服务器认证的
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},               // 密钥扩展用途的序列
		EmailAddresses: []string{"forward.nice.cp@gmail.com"},
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}

	priKey := rootKey

	cer, err := x509.CreateCertificate(rand.Reader, &template, rootCa, &priKey.PublicKey, rootKey)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{ // 证书
			Type:  "CERTIFICATE",
			Bytes: cer,
		}), pem.EncodeToMemory(&pem.Block{ // 私钥
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priKey),
		}), err
}

// 加载根Private Key
func loadRootKey(Key []byte) (*rsa.PrivateKey, error) {
	p, _ := pem.Decode(Key)
	if p == nil {
		return nil, errors.New("parse Key Fail ")
	}
	rootKey, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		k, e := x509.ParsePKCS8PrivateKey(p.Bytes)
		if e != nil {
			return nil, errors.New(err.Error() + " or " + e.Error())
		}
		kk := k.(*rsa.PrivateKey)
		if kk == nil {
			return nil, err
		}
		rootKey = kk
	}
	return rootKey, nil
}

func (c *CertManager) GetCommonName() string {
	certDERBlock, _ := pem.Decode([]byte(c.Certificates))
	x509Cert, _ := x509.ParseCertificate(certDERBlock.Bytes)
	if x509Cert != nil && x509Cert.Subject.CommonName != "" {
		return x509Cert.Subject.CommonName
	}
	return ""
}

// 加载根证书
func (c *CertManager) loadRootCa(Ca []byte) (*x509.Certificate, error) {
	if c.Tls == nil {
		return nil, errors.New("CertManagerOBJ=Null")
	}
	p, _ := pem.Decode(Ca)
	if p == nil {
		return nil, errors.New("parse ca Fail ")
	}
	rootCa, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return nil, err
	}
	return rootCa, nil
}

func (c *CertManager) CreateCA(Country, Organization, OrganizationalUnit, Province, CommonName, Locality string, bits, NotAfter int) bool {
	if c.Tls == nil {
		return false
	}
	cKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return false
	}
	serialNumber, _ := rand.Int(rand.Reader, public.MaxBig)
	template := &x509.Certificate{
		SerialNumber: serialNumber, // SerialNumber 是 CA 颁布的唯一序列号，在此使用一个大随机数来代表它
		Subject: pkix.Name{ // 证书的主题信息
			Country:            []string{Country},            // 证书所属的国家
			Organization:       []string{Organization},       // 证书存放的公司名称
			OrganizationalUnit: []string{OrganizationalUnit}, // 证书所属的部门名称
			Province:           []string{Province},           // 证书签发机构所在省
			CommonName:         CommonName,                   // 证书域名
			Locality:           []string{Locality},           // 证书签发机构所在市
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, NotAfter),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, // 典型用法是指定叶子证书中的公钥的使用目的。它包括一系列的OID，每一个都指定一种用途。例如{id pkix 31}表示用于服务器端的TLS/SSL连接；{id pkix 34}表示密钥可以用于保护电子邮件。
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,                      // 指定了这份证书包含的公钥可以执行的密码操作，例如只能用于签名，但不能用来加密
		IsCA:                  true,                                                                       // 指示证书是不是ca证书
		BasicConstraintsValid: true,                                                                       // 指示证书是不是ca证书
	}
	rootCertDer, err := x509.CreateCertificate(rand.Reader, template, template, &cKey.PublicKey, cKey) //DER 格式
	if err != nil {
		return false
	}
	caProvBytes := x509.MarshalPKCS1PrivateKey(cKey)
	rootKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: caProvBytes})
	if len(rootKey) < 1 {
		return false
	}
	rootCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCertDer})
	if len(rootKey) < 1 {
		return false
	}
	c.PrivateKey = string(rootKey)
	c.Certificates = string(rootCert)
	c.setCert(rootKey, rootCert)
	_ = c.LoadX509Certificate(CommonName, string(rootCert), string(rootKey))
	return true
}

func CertToP12(certBuf, keyBuf, Pwd string) (p12Cert []byte, err error) {

	caBlock, _ := pem.Decode([]byte(certBuf))
	crt, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		err = fmt.Errorf("证书解析异常, Error : %v", err)
		return
	}

	keyBlock, _ := pem.Decode([]byte(keyBuf))
	priKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		k, e := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if e != nil {
			err = fmt.Errorf("证书密钥解析key异常, Error : %v", err)
			return
		}
		kk := k.(*rsa.PrivateKey)
		if kk == nil {
			err = fmt.Errorf("证书密钥解析key异常, Error : %v", err)
			return
		}
		priKey = kk
	}

	pfx, err := pkcs.Encode(rand.Reader, priKey, crt, nil, Pwd)
	if err != nil {
		err = fmt.Errorf("pem to p12 转换证书异常, Error : %v", err)
		return
	}

	return pfx, err

}

// ExportCA 证书管理器 导出证书
func (c *CertManager) ExportCA() string {
	return c.Certificates
}

// ExportKEY 证书管理器 导出私钥
func (c *CertManager) ExportKEY() string {
	return c.PrivateKey
}

// ExportPub 证书管理器 导出公钥
func (c *CertManager) ExportPub() string {
	k := c.PrivateKey
	if k == public.NULL {
		return public.NULL
	}
	p, _ := pem.Decode([]byte(k))
	if p == nil {
		return public.NULL
	}
	Key, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		kc, e := x509.ParsePKCS8PrivateKey(p.Bytes)
		if e != nil {
			return public.NULL
		}
		kk := kc.(*rsa.PrivateKey)
		if kk == nil {
			return public.NULL
		}
		Key = kk
	}
	pubs, err := x509.MarshalPKIXPublicKey(&Key.PublicKey)
	if err != nil {
		return public.NULL
	}
	rootPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubs})
	return string(rootPub)
}

// ExportP12 证书管理器 导出为P12
func (c *CertManager) ExportP12(path, pass string) bool {
	CA := c.ExportCA()
	k := c.PrivateKey
	if CA == public.NULL || k == public.NULL {
		return false
	}
	b, e := CertToP12(CA, k, pass)
	if e != nil {
		return false
	}
	e = public.WriteBytesToFile(b, path)
	return e == nil
}
