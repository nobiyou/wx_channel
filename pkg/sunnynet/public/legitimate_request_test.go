package public

import "testing"

func TestLegitimateRequest_NoBodyWithoutContentLengthIsComplete(t *testing.T) {
	for _, method := range []string{"GET", "DELETE", "PUT", "POST"} {
		req := []byte(method + " /api/queue/x HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")
		_, ok, _, _, isHTTP := LegitimateRequest(req)
		if !isHTTP || !ok {
			t.Fatalf("%s without Content-Length should be complete, ok=%v isHTTP=%v", method, ok, isHTTP)
		}
	}
	// 头部尚未读完：不应判定为完整
	if _, ok, _, _, _ := LegitimateRequest([]byte("DELETE /x HTTP/1.1\r\nHost: a\r\n")); ok {
		t.Fatal("incomplete headers should not be complete")
	}
	// 有 Content-Length 但 Body 未到齐：不应判定为完整
	if _, ok, _, _, _ := LegitimateRequest([]byte("POST /x HTTP/1.1\r\nContent-Length: 5\r\n\r\nab")); ok {
		t.Fatal("partial body should not be complete")
	}
}
