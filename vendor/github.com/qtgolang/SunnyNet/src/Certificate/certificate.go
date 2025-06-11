package Certificate

import (
	"crypto/tls"
	"encoding/pem"
	"errors"
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/crypto/pkcs"
	"io/ioutil"
	"os"
	"strings"
	"sync"
)

func AddP12Certificate(privateKeyName, privatePassword string) (*tls.Certificate, string, string, string, error) {
	PRIVATE := ""
	Certificates := ""
	k, e := getPrivateKey(privateKeyName, privatePassword)
	if k == nil {
		return nil, Certificates, PRIVATE, public.NULL, errors.New("Loading P12 Error  :" + e.Error())
	}
	var pemData []byte
	for _, b := range k {
		if strings.Index(b.Type, "PRIVATE") != -1 {
			PRIVATE = string(pem.EncodeToMemory(b))
		} else if strings.Index(b.Type, "CERTIFICATE") != -1 {
			Certificates = string(pem.EncodeToMemory(b))
		}
		pemData = append(pemData, pem.EncodeToMemory(b)...)
	}
	ce, err := tls.X509KeyPair(pemData, pemData)
	if err != nil {
		return nil, Certificates, PRIVATE, public.NULL, errors.New("Loading P12 Error  :" + err.Error())
	}

	return &ce, Certificates, PRIVATE, string(pemData), nil
}

func getPrivateKey(privateKeyName, privatePassword string) ([]*pem.Block, error) {
	f, err := os.Open(privateKeyName)
	if err != nil {
		return nil, err
	}

	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	A, C := pkcs.ToPEM(bytes, privatePassword)
	if A == nil {
		return nil, C
	}
	return A, C
}

// 储存管理 MessageId
//  ---------------------------------------------

var MessageIdLock sync.Mutex
var messageId = 1000

// 创建新的 messageId
func newMessageId() int {
	MessageIdLock.Lock()
	defer MessageIdLock.Unlock()
	messageId++
	t := messageId
	if t < 0 || t > 2147483647 {
		t = 9999
		messageId = 1000
	}
	return t
}
