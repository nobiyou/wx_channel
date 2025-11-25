package SunnyNet

type ConnHTTP interface {
	GetURL() string
	GetMethod() string
	GetHost() string
	GetRequestHeader() map[string][]string
	GetResponseHeader() map[string][]string
	GetRequestBody() []byte
	GetResponseBody() []byte
	SetResponseBody([]byte)
	Block()
	GetURLPath() string
}

type Sunny struct {
	Error error
	port  int
}

func NewSunny() *Sunny {
	return &Sunny{}
}

func (s *Sunny) SetPort(port int) *Sunny {
	s.port = port
	return s
}

func (s *Sunny) Start() *Sunny {
	return s
}

func (s *Sunny) SetGoCallback(callback func(ConnHTTP), _ interface{}, _ interface{}, _ interface{}) *Sunny {
	return s
}

func (s *Sunny) OpenDrive(flag bool) bool {
	return true
}

func (s *Sunny) ProcessAddName(name string) {
	// Do nothing
}
