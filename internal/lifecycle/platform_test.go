package lifecycle

import "testing"

func TestParseTasklistOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "wechat process", raw: `"Weixin.exe","18260","Console","1","120,000 K"`, want: true},
		{name: "unrelated process", raw: `"chrome.exe","17332","Console","1","120,000 K"`, want: false},
		{name: "empty", raw: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTasklistOutput([]byte(tt.raw))
			if err != nil {
				t.Fatalf("parseTasklistOutput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseTasklistOutput() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestParseTasklistOutputRejectsMalformedCSV(t *testing.T) {
	if _, err := parseTasklistOutput([]byte(`"Weixin.exe","18260`)); err == nil {
		t.Fatal("parseTasklistOutput() error = nil, want malformed CSV error")
	}
}
