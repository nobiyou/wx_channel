//go:build windows

package lifecycle

import (
	"context"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	wechatWindowTitle = "微信"
	wechatAppExClass  = "Chrome_WidgetWin_0"

	// WeChat 4.x renders its sidebar inside the client area. These are the
	// logical client coordinates of the video-channel entry on the current
	// desktop layout; the DPI scale is applied before the click.
	videoChannelSidebarX int32 = 30
	videoChannelSidebarY int32 = 306

	swRestore         = 9
	mouseEventDown    = 0x0002
	mouseEventUp      = 0x0004
	defaultDPI        = 96
	activationTimeout = 2 * time.Second
	activationPoll    = 25 * time.Millisecond
)

var user32 = syscall.NewLazyDLL("user32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	procFindWindowW         = user32.NewProc("FindWindowW")
	procShowWindowAsync     = user32.NewProc("ShowWindowAsync")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadID   = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procSetActiveWindow     = user32.NewProc("SetActiveWindow")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procClientToScreen      = user32.NewProc("ClientToScreen")
	procGetDPIForWindow     = user32.NewProc("GetDpiForWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetCursorPos        = user32.NewProc("SetCursorPos")
	procMouseEvent          = user32.NewProc("mouse_event")
	procGetCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
)

type windowsPoint struct {
	X int32
	Y int32
}

type windowsRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type sidebarPageOpener struct{}

func newPageOpener() PageOpener {
	return sidebarPageOpener{}
}

// Open activates the existing WeChat window and performs the same sidebar
// action a user uses to enter Channels. URI handlers vary across WeChat
// desktop builds and only spawned a helper process on the observed build.
func (sidebarPageOpener) Open(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	hwnd, err := findWeChatWindow()
	if err != nil {
		return err
	}

	requestForeground(hwnd)
	if err := waitForActivation(ctx, hwnd); err != nil {
		return err
	}

	client, err := clientRect(hwnd)
	if err != nil {
		return err
	}
	point := videoChannelSidebarPoint(hwnd)
	if point.X <= client.Left || point.X >= client.Right || point.Y <= client.Top || point.Y >= client.Bottom {
		return fmt.Errorf("video channel sidebar point (%d,%d) outside WeChat client %dx%d", point.X, point.Y, client.Right-client.Left, client.Bottom-client.Top)
	}

	return clickScreenPoint(ctx, hwnd, point)
}

func findWeChatWindow() (syscall.Handle, error) {
	// The visible Channels page is hosted by WeChatAppEx.exe in a Chrome
	// widget. Find it before the Qt shell window, because both use the title
	// "微信" on current WeChat builds.
	if hwnd := findWindowHandle(wechatAppExClass, wechatWindowTitle); hwnd != 0 {
		return hwnd, nil
	}
	if hwnd := findWindowHandle("", wechatWindowTitle); hwnd != 0 {
		return hwnd, nil
	}
	return 0, fmt.Errorf("find WeChat window %q: not found", wechatWindowTitle)
}

func findWindowHandle(className, title string) syscall.Handle {
	var classPtr, titlePtr uintptr
	if className != "" {
		class := syscall.StringToUTF16Ptr(className)
		classPtr = uintptr(unsafe.Pointer(class))
	}
	if title != "" {
		windowTitle := syscall.StringToUTF16Ptr(title)
		titlePtr = uintptr(unsafe.Pointer(windowTitle))
	}
	hwnd, _, _ := procFindWindowW.Call(classPtr, titlePtr)
	return syscall.Handle(hwnd)
}

func waitForActivation(ctx context.Context, want syscall.Handle) error {
	timeout := time.NewTimer(activationTimeout)
	poll := time.NewTicker(activationPoll)
	defer timeout.Stop()
	defer poll.Stop()

	for {
		if foreground := procGetForegroundWindowHandle(); foreground == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("activate WeChat window: foreground=%v want=%v", procGetForegroundWindowHandle(), want)
		case <-poll.C:
			requestForeground(want)
		}
	}
}

// requestForeground handles the Windows foreground-lock rule. A background
// process may call SetForegroundWindow successfully yet still lose the race
// to the current foreground thread; temporarily sharing that input queue
// makes the same activation path work when wx_channel is started elevated or
// from a launcher window.
func requestForeground(hwnd syscall.Handle) {
	procShowWindowAsync.Call(uintptr(hwnd), swRestore)
	foreground := procGetForegroundWindowHandle()
	currentThreadValue, _, _ := procGetCurrentThreadID.Call()
	currentThread := uint32(currentThreadValue)
	foregroundThread := windowThreadID(foreground)
	attached := false
	if currentThread != 0 && foregroundThread != 0 && currentThread != foregroundThread {
		attachedValue, _, _ := procAttachThreadInput.Call(uintptr(currentThread), uintptr(foregroundThread), 1)
		attached = attachedValue != 0
	}
	if attached {
		defer procAttachThreadInput.Call(uintptr(currentThread), uintptr(foregroundThread), 0)
	}

	procBringWindowToTop.Call(uintptr(hwnd))
	procSetActiveWindow.Call(uintptr(hwnd))
	procSetForegroundWindow.Call(uintptr(hwnd))
}

func windowThreadID(hwnd syscall.Handle) uint32 {
	if hwnd == 0 {
		return 0
	}
	threadID, _, _ := procGetWindowThreadID.Call(uintptr(hwnd), 0)
	return uint32(threadID)
}

func procGetForegroundWindowHandle() syscall.Handle {
	hwnd, _, _ := procGetForegroundWindow.Call()
	return syscall.Handle(hwnd)
}

func clientRect(hwnd syscall.Handle) (windowsRect, error) {
	var rect windowsRect
	if result, _, _ := procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect))); result == 0 {
		return windowsRect{}, fmt.Errorf("read WeChat client rectangle")
	}
	return rect, nil
}

func videoChannelSidebarPoint(hwnd syscall.Handle) windowsPoint {
	dpi := defaultDPI
	if value, _, _ := procGetDPIForWindow.Call(uintptr(hwnd)); value != 0 {
		dpi = int(value)
	}
	return windowsPoint{
		X: videoChannelSidebarX * int32(dpi) / defaultDPI,
		Y: videoChannelSidebarY * int32(dpi) / defaultDPI,
	}
}

func clickScreenPoint(ctx context.Context, hwnd syscall.Handle, point windowsPoint) error {
	var original windowsPoint
	if result, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&original))); result == 0 {
		return fmt.Errorf("read cursor position")
	}

	screenPoint := point
	if result, _, _ := procClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&screenPoint))); result == 0 {
		return fmt.Errorf("convert WeChat client point to screen")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if result, _, _ := procSetCursorPos.Call(uintptr(screenPoint.X), uintptr(screenPoint.Y)); result == 0 {
		return fmt.Errorf("move cursor to video channel entry")
	}
	defer procSetCursorPos.Call(uintptr(original.X), uintptr(original.Y))

	procMouseEvent.Call(mouseEventDown, 0, 0, 0, 0)
	procMouseEvent.Call(mouseEventUp, 0, 0, 0, 0)
	return nil
}
