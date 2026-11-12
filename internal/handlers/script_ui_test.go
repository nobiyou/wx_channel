package handlers

import (
	"strings"
	"testing"
)

func TestBuildInjectedScriptsOmitsRemovedLogPanel(t *testing.T) {
	script := (&ScriptHandler{}).buildInjectedScripts("/web/pages/home")
	for _, marker := range []string{
		"__wx_channels_log_panel",
		"__wx_channels_log_toggle_btn",
		"WX_CHANNEL_SHOW_LOG_BUTTON",
	} {
		if strings.Contains(script, marker) {
			t.Fatalf("injected page script still contains removed log-panel marker %q", marker)
		}
	}
}
