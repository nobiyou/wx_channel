package poc

import (
	"bytes"
	"encoding/json"
)

var approvedPagePaths = map[string]struct{}{
	"/web/pages/home":    {},
	"/web/pages/feed":    {},
	"/web/pages/profile": {},
}

type BridgeConfig struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

func InjectHTML(host, path string, input, script []byte, config BridgeConfig) ([]byte, bool) {
	if host != "channels.weixin.qq.com" {
		return input, false
	}
	if _, ok := approvedPagePaths[path]; !ok || config.Port < 1 || config.Port > 65535 || config.Token == "" || len(script) == 0 {
		return input, false
	}
	lower := bytes.ToLower(input)
	head := bytes.Index(lower, []byte("<head>"))
	if head < 0 {
		return input, false
	}
	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return input, false
	}
	injection := make([]byte, 0, len(encodedConfig)+len(script)+128)
	injection = append(injection, []byte(`<script>window.__WX_CHANNEL_POC_CONFIG__=`)...)
	injection = append(injection, encodedConfig...)
	injection = append(injection, []byte(`;</script><script>`)...)
	injection = append(injection, script...)
	injection = append(injection, []byte(`</script>`)...)
	position := head + len("<head>")
	result := make([]byte, 0, len(input)+len(injection))
	result = append(result, input[:position]...)
	result = append(result, injection...)
	result = append(result, input[position:]...)
	return result, true
}
