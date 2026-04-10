package SunnyNet

import "net/http"

// HttpConn represents a proxied HTTP connection.
// Compatible with the original SunnyNet HttpConn type.
type HttpConn struct {
	Type     int
	Request  *http.Request
	Response *http.Response

	// Internal: whether StopRequest was called
	stopped    bool
	stopCode   int
	stopBody   string
	stopHeader http.Header
}

// StopRequest halts further processing and sends a synthetic response.
func (c *HttpConn) StopRequest(statusCode int, body string, headers http.Header) {
	c.stopped = true
	c.stopCode = statusCode
	c.stopBody = body
	c.stopHeader = headers
}

// IsStopped returns whether StopRequest was called.
func (c *HttpConn) IsStopped() bool {
	return c.stopped
}
