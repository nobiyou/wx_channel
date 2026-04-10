package public

// HTTP callback type constants, matching the original SunnyNet public package.
const (
	HttpSendRequest = 1 // Fired when client sends a request (before forwarding)
	HttpResponseOK  = 2 // Fired when server responds successfully
)

// Free is a no-op placeholder for SunnyNet compatibility.
func Free(v interface{}) {
	// No-op
}
