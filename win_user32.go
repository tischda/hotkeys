//go:build windows

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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
	findWindowExW    = user32.NewProc("FindWindowExW")
	destroyWindow    = user32.NewProc("DestroyWindow")

	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const swHide = 0

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
const hotkeyWindowClassName = "HotkeyWindow"

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

// Theres should only be one instance of the hotkeys server running at a time.
// We use a named mutex to enforce this.
const singleInstanceMutexName = `Local\Hotkeys.SingleInstance`
const gracefulStopEventName = `Local\Hotkeys.GracefulStop`

var errAlreadyRunning = errors.New("another hotkeys instance is already running")

func acquireSingleInstanceLock() (func(), error) {
	mutexName, err := syscall.UTF16PtrFromString(singleInstanceMutexName)
	if err != nil {
		return nil, fmt.Errorf("mutex name utf16: %w", err)
	}

	handle, createErr := windows.CreateMutex(nil, false, mutexName)
	if handle == 0 {
		return nil, fmt.Errorf("CreateMutex: %w", createErr)
	}

	if createErr != nil {
		if errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
			_ = windows.CloseHandle(handle) //nolint:errcheck
			return nil, errAlreadyRunning
		}
		_ = windows.CloseHandle(handle) //nolint:errcheck
		return nil, fmt.Errorf("CreateMutex: %w", createErr)
	}

	release := func() {
		_ = windows.CloseHandle(handle) //nolint:errcheck
	}
	return release, nil
}

// startGracefulStopListener waits for a named stop event and invokes onStop once.
func startGracefulStopListener(onStop func()) (func(), error) {
	if onStop == nil {
		onStop = func() {}
	}

	eventHandle, err := createOrOpenGracefulStopEvent()
	if err != nil {
		return nil, err
	}

	go func() {
		_, waitErr := windows.WaitForSingleObject(eventHandle, windows.INFINITE)
		if waitErr != nil {
			logger.Printf("Graceful stop wait failed: %v", waitErr)
			return
		}
		onStop()
	}()

	cleanup := func() {
		_ = windows.CloseHandle(eventHandle) //nolint:errcheck
	}

	return cleanup, nil
}

// signalGracefulStop notifies the running hotkeys process to shutdown gracefully.
func signalGracefulStop() error {
	if err := signalGracefulStopEvent(); err == nil {
		return nil
	}

	// Backward compatibility: older running instances may not expose the stop
	// event. In that case, post the quit message to the hotkeys message window.
	if err := signalGracefulStopWindow(); err == nil {
		return nil
	}

	return signalGracefulStopEvent()
}

func signalGracefulStopEvent() error {
	eventName, err := syscall.UTF16PtrFromString(gracefulStopEventName)
	if err != nil {
		return fmt.Errorf("stop event name utf16: %w", err)
	}

	eventHandle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName)
	if err != nil {
		return fmt.Errorf("open stop event: %w", err)
	}
	defer windows.CloseHandle(eventHandle) //nolint:errcheck

	if err := windows.SetEvent(eventHandle); err != nil {
		return fmt.Errorf("set stop event: %w", err)
	}

	return nil
}

func signalGracefulStopWindow() error {
	className, err := syscall.UTF16PtrFromString(hotkeyWindowClassName)
	if err != nil {
		return fmt.Errorf("window class utf16: %w", err)
	}

	hwnd, _, findErr := findWindowExW.Call(HWND_MESSAGE, 0, uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		return fmt.Errorf("find window: %v", findErr)
	}

	posted, _, postErr := postMessageW.Call(hwnd, WM_APP_QUIT, 0, 0)
	if posted == 0 {
		return fmt.Errorf("post quit message: %v", postErr)
	}

	return nil
}

func createOrOpenGracefulStopEvent() (windows.Handle, error) {
	eventName, err := syscall.UTF16PtrFromString(gracefulStopEventName)
	if err != nil {
		return 0, fmt.Errorf("stop event name utf16: %w", err)
	}

	eventHandle, createErr := windows.CreateEvent(nil, 1, 0, eventName)
	if eventHandle == 0 {
		return 0, fmt.Errorf("create stop event: %w", createErr)
	}

	if createErr != nil && !errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(eventHandle) //nolint:errcheck
		return 0, fmt.Errorf("create stop event: %w", createErr)
	}

	if err := windows.ResetEvent(eventHandle); err != nil {
		_ = windows.CloseHandle(eventHandle) //nolint:errcheck
		return 0, fmt.Errorf("reset stop event: %w", err)
	}

	return eventHandle, nil
}

// findRunningProcessID finds a running process for imageName, excluding currentPID.
//
// Parameters:
//   - imageName: Process image name to match (for example, hotkeys.exe).
//   - currentPID: Process ID to ignore from the results.
//
// Returns:
//   - int: PID of a matching process when found, otherwise 0.
//   - bool: True when a matching process PID is found.
//   - error: Non-nil when Win32 process enumeration fails.
func findRunningProcessID(imageName string, currentPID int) (int, bool, error) {
	if strings.TrimSpace(imageName) == "" {
		return 0, false, nil
	}
	targetImageName := filepath.Base(strings.TrimSpace(imageName))

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, false, fmt.Errorf("create process snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot) //nolint:errcheck

	var processEntry windows.ProcessEntry32
	processEntry.Size = uint32(unsafe.Sizeof(processEntry))
	if err := windows.Process32First(snapshot, &processEntry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("enumerate first process: %w", err)
	}

	for {
		entryImageName := windows.UTF16ToString(processEntry.ExeFile[:])
		if int(processEntry.ProcessID) != currentPID && isProcessImageMatch(entryImageName, targetImageName) {
			return int(processEntry.ProcessID), true, nil
		}

		err = windows.Process32Next(snapshot, &processEntry)
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return 0, false, nil
			}
			return 0, false, fmt.Errorf("enumerate next process: %w", err)
		}
	}
}

// isProcessImageMatch performs a case-insensitive match on executable image names.
func isProcessImageMatch(candidateImageName, targetImageName string) bool {
	candidate := filepath.Base(strings.TrimSpace(candidateImageName))
	target := filepath.Base(strings.TrimSpace(targetImageName))
	if candidate == "" || target == "" {
		return false
	}
	return strings.EqualFold(candidate, target)
}
