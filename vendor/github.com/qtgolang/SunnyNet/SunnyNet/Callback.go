package SunnyNet

import "C"
import (
	"github.com/qtgolang/SunnyNet/Call"
	"github.com/qtgolang/SunnyNet/public"
	"strconv"
)

func GetSceneProxyRequest(MessageId int) (*ProxyRequest, bool) {
	messageIdLock.Lock()
	defer messageIdLock.Unlock()
	k := httpStorage[MessageId]
	if k == nil {
		return nil, false
	}
	return k, true
}
func GetSceneWebSocketMsg(MessageId int) (*public.WebsocketMsg, bool) {
	messageIdLock.Lock()
	defer messageIdLock.Unlock()
	k := wsStorage[MessageId]
	if k == nil {
		return nil, false
	}
	return k, true
}

// CallbackTCPRequest TCP请求处理回调
func (s *ProxyRequest) CallbackTCPRequest(callType int, msg *public.TcpMsg) {
	if s.Global.disableTCP {
		//由于用户可能在软件中途禁用TCP,所有这里允许触发关闭的回调
		if callType != public.SunnyNetMsgTypeTCPClose {
			//这里如果禁用了TCP,那么这里就不允许触发回调了，并且手动关闭连接
			TcpSceneLock.Lock()
			w := TcpStorage[s.Theology]
			TcpSceneLock.Unlock()
			if w == nil {
				return
			}
			w.L.Lock()
			_ = w.ConnSend.Close()
			_ = w.ConnServer.Close()
			w.L.Unlock()
			return
		}
	}
	LocalAddr := s.Conn.RemoteAddr().String()
	hostname := s.Target.String()
	pid, _ := strconv.Atoi(s.Pid)
	MessageId := NewMessageId()
	if s.TcpCall < 10 {
		if s.TcpGoCall != nil {
			Ams := &TcpConn{c: msg, MessageId: MessageId, Type: callType, Theology: s.Theology, LocalAddr: LocalAddr, RemoteAddr: hostname, Pid: pid, SunnyContext: s.Global.SunnyContext}
			s.TcpGoCall(Ams)
		}
		return
	}
	messageIdLock.Lock()
	httpStorage[MessageId] = s
	messageIdLock.Unlock()

	if callType != public.SunnyNetMsgTypeTCPClientSend && callType != public.SunnyNetMsgTypeTCPClientReceive && callType != public.SunnyNetMsgTypeTCPAboutToConnect {
		Call.Call(s.TcpCall, s.Global.SunnyContext, LocalAddr, hostname, callType, MessageId, []byte{}, 0, s.Theology, pid)
		messageIdLock.Lock()
		httpStorage[MessageId] = nil
		delete(httpStorage, MessageId)
		messageIdLock.Unlock()
		return
	}
	if callType == public.SunnyNetMsgTypeTCPClientSend || callType == public.SunnyNetMsgTypeTCPAboutToConnect {
		s.TCP.Send = msg
	} else {
		s.TCP.Receive = msg
	}
	Call.Call(s.TcpCall, s.Global.SunnyContext, LocalAddr, hostname, callType, MessageId, msg.Data.Bytes(), msg.Data.Len(), s.Theology, pid)

	messageIdLock.Lock()
	httpStorage[MessageId] = nil
	delete(httpStorage, MessageId)
	messageIdLock.Unlock()
}

// CallbackBeforeRequest HTTP发起请求处理回调
func (s *ProxyRequest) CallbackBeforeRequest() {

	if s.Response != nil {
		if s.Response.Body != nil {
			_ = s.Response.Body.Close()
		}
	}
	s.Response = nil
	MessageId := NewMessageId()
	pid, _ := strconv.Atoi(s.Pid)
	if s.HttpCall < 10 {
		if s.HttpGoCall != nil {
			m := &HttpConn{Theology: s.Theology, MessageId: MessageId, PID: pid, SunnyContext: s.Global.SunnyContext, Type: public.HttpSendRequest, Request: s.Request, Response: nil, err: "", proxy: s.Proxy, ClientIP: s.Conn.RemoteAddr().String()}
			s.HttpGoCall(m)
			s.Response = m.Response
		}
		return
	}
	Method := s.Request.Method
	Url := s.Request.URL.String()
	messageIdLock.Lock()
	httpStorage[MessageId] = s
	messageIdLock.Unlock()

	Call.Call(s.HttpCall, s.Global.SunnyContext, s.Theology, MessageId, public.HttpSendRequest, Method, Url, "", pid)

	messageIdLock.Lock()
	httpStorage[MessageId] = nil
	delete(httpStorage, MessageId)
	messageIdLock.Unlock()

}

// CallbackBeforeResponse HTTP请求完成处理回调
func (s *ProxyRequest) CallbackBeforeResponse() {
	pid, _ := strconv.Atoi(s.Pid)
	if s.HttpCall < 10 {
		if s.HttpGoCall != nil {
			m := &HttpConn{Theology: s.Theology, MessageId: NewMessageId(), PID: pid, SunnyContext: s.Global.SunnyContext, Type: public.HttpResponseOK, Request: s.Request, Response: s.Response, err: "", ClientIP: s.Conn.RemoteAddr().String()}
			s.HttpGoCall(m)
		}
		return
	}
	MessageId := NewMessageId()
	Method := s.Request.Method
	Url := s.Request.URL.String()

	messageIdLock.Lock()
	httpStorage[MessageId] = s
	messageIdLock.Unlock()

	Call.Call(s.HttpCall, s.Global.SunnyContext, s.Theology, MessageId, public.HttpResponseOK, Method, Url, "", pid)

	messageIdLock.Lock()
	httpStorage[MessageId] = nil
	delete(httpStorage, MessageId)
	messageIdLock.Unlock()
}

// CallbackWssRequest HTTP->Websocket请求处理回调
func (s *ProxyRequest) CallbackWssRequest(State int, Method, Url string, msg *public.WebsocketMsg, MessageId int) {
	pid, _ := strconv.Atoi(s.Pid)
	//Websocket消息
	if s.wsCall < 10 {
		if s.wsGoCall != nil {
			m := &WsConn{Pid: pid, Type: State, SunnyContext: s.Global.SunnyContext, Url: Url, c: msg, MessageId: MessageId, Theology: s.Theology, Request: s.Request, ClientIP: s.Conn.RemoteAddr().String()}
			s.wsGoCall(m)
		}
		return
	}
	Call.Call(s.wsCall, s.Global.SunnyContext, s.Theology, MessageId, State, Method, Url, pid, msg.Mt)
}

// CallbackError HTTP请求失败处理回调
func (s *ProxyRequest) CallbackError(err error) {
	pid, _ := strconv.Atoi(s.Pid)
	if s.HttpCall < 10 {
		if s.HttpGoCall != nil {
			m := &HttpConn{Theology: s.Theology, MessageId: NewMessageId(), PID: pid, SunnyContext: s.Global.SunnyContext, Type: public.HttpRequestFail, Request: s.Request, Response: nil, err: err.Error(), ClientIP: s.Conn.RemoteAddr().String()}
			s.HttpGoCall(m)
		}
		return
	}
	//请求失败
	MessageId := NewMessageId()
	Method := s.Request.Method

	Url := "Unknown URL"
	if s.Request.URL != nil {
		Url = s.Request.URL.String()
	}

	messageIdLock.Lock()
	httpStorage[MessageId] = s
	messageIdLock.Unlock()

	Call.Call(s.HttpCall, s.Global.SunnyContext, s.Theology, MessageId, public.HttpRequestFail, Method, Url, err.Error(), pid)

	messageIdLock.Lock()
	httpStorage[MessageId] = nil
	delete(httpStorage, MessageId)
	messageIdLock.Unlock()

}
