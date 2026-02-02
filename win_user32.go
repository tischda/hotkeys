//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	registerHotKey   = user32.NewProc("RegisterHotKey")
	unregisterHotKey = user32.NewProc("UnregisterHotKey")

	getMessageW      = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessageW = user32.NewProc("DispatchMessageW")
	postMessageW     = user32.NewProc("PostMessageW")
	postQuitMessage  = user32.NewProc("PostQuitMessage")

	defWindowProcW   = user32.NewProc("DefWindowProcW")
	registerClassExW = user32.NewProc("RegisterClassExW")
	createWindowExW  = user32.NewProc("CreateWindowExW")
	destroyWindow    = user32.NewProc("DestroyWindow")

	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

const HWND_MESSAGE uintptr = ^uintptr(2) // 0xFFFFFFFFFFFFFFFD
const WM_HOTKEY = 0x0312

const WM_APP = 0x8000
const WM_APP_RELOAD = WM_APP + 1
const WM_APP_QUIT = WM_APP + 2

type WNDCLASSEX struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

// wndProc handles window messages for the hidden message-only window.
//
// Parameters:
//   - hwnd: Handle to the message-only window.
//   - msg: Windows message ID.
//   - wparam: Message-specific WPARAM value.
//   - lparam: Message-specific LPARAM value.
//
// Returns:
//   - uintptr: The result expected by Windows for the given message.
func wndProc(hwnd syscall.Handle, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_HOTKEY:
		id := uint32(wparam)
		for _, hk := range hotkeys {
			if hk.Id == id {
				logger.Printf("Executing: %v", hk.Action)
				if _, err := executeCommand(hk.Action); err != nil {
					logger.Println("ERROR:", err)
				}
				break
			}
		}
	case WM_APP_RELOAD:
		if err := reloadHotkeys(uintptr(hwnd)); err != nil {
			logger.Printf("Failed to load config %s: %v", configPath, err)
		}
	case WM_APP_QUIT:
		postQuitMessage.Call(0) //nolint:errcheck
		return 0
	default:
		// Call default window procedure for unhandled messages
		r, _, _ := defWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
		return r
	}
	return 0
}

// createHiddenWindow creates a message-only window registered with className.
//
// Parameters:
//   - className: Window class name to register and instantiate.
//
// Returns:
//   - uintptr: Handle to the created window.
//   - error: Non-nil if the class cannot be registered or the window cannot be created.
func createHiddenWindow(className string) (uintptr, error) {
	classNamePtr, err := syscall.UTF16PtrFromString(className)
	if err != nil {
		return 0, err
	}

	// 1. Get current module instance
	instance, _, err := getModuleHandleW.Call(0)
	if instance == 0 {
		return 0, err
	}

	// 2. Register window class
	wc := WNDCLASSEX{
		Size:      uint32(unsafe.Sizeof(WNDCLASSEX{})),
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  syscall.Handle(instance),
		ClassName: classNamePtr,
	}
	atom, _, err := registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return 0, err
	}

	// 3. Create the hidden message-only window
	hwnd, _, lastErr := createWindowExW.Call(
		0, uintptr(atom), 0, 0, 0, 0, 0, 0,
		HWND_MESSAGE,
		0, instance, 0,
	)
	if hwnd == 0 {
		return 0, lastErr
	}
	return hwnd, nil
}

// registerAll registers all configured hotkeys for hwnd.
//
// Parameters:
//   - hwnd: Handle to the message-only window to register hotkeys against.
func registerAll(hwnd uintptr) {
	for _, hk := range hotkeys {
		r1, _, err := registerHotKey.Call(hwnd, uintptr(hk.Id), uintptr(hk.Modifiers), uintptr(hk.KeyCode))
		if r1 == 0 {
			logger.Printf("Failed to register hotkey %d (%s): %v", hk.Id, hk.KeyString, err)
		} else {
			logger.Printf("Registered %d: %s -> %v", hk.Id, hk.KeyString, hk.Action)
		}
	}
}

// unregisterAll unregisters all configured hotkeys for hwnd.
//
// Parameters:
//   - hwnd: Handle to the message-only window whose hotkeys should be unregistered.
func unregisterAll(hwnd uintptr) {
	if hotkeys == nil {
		return
	}
	for _, hk := range hotkeys {
		unregisterHotKey.Call(hwnd, uintptr(hk.Id)) //nolint:errcheck
	}
	logger.Println("Unregistered all hotkeys.")
}

// messageLoop runs the Windows message loop until WM_QUIT is received.
func messageLoop() {
	var msg MSG
	for {
		r, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == 0 {
			break
		}
		if int32(r) == -1 {
			logger.Printf("GetMessage error: %v", syscall.GetLastError())
			continue
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg))) //nolint:errcheck
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&msg))) //nolint:errcheck
	}
}
