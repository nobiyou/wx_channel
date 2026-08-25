//go:build windows

package lifecycle

import "testing"

func TestVideoChannelSidebarPointUsesCurrentWeChatEntry(t *testing.T) {
	point := videoChannelSidebarPoint(0)
	if point.X != videoChannelSidebarX || point.Y != videoChannelSidebarY {
		t.Fatalf("sidebar point = (%d,%d), want (%d,%d)", point.X, point.Y, videoChannelSidebarX, videoChannelSidebarY)
	}
}
