// The helper owns one registered chord. While held, check only its trigger and
// modifiers for release. Closing stdin also ends registration if Desktop exits.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")

func virtualKey(name string) uintptr {
	named := map[string]uintptr{"Space": 32, "Enter": 13, "Left": 37, "Up": 38, "Right": 39, "Down": 40}
	if key := named[name]; key != 0 {
		return key
	}
	if len(name) > 1 && strings.HasPrefix(name, "F") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 1 && n <= 12 {
			return uintptr(111 + n)
		}
	}
	if len(name) != 1 {
		return 0
	}
	key, _, _ := user32.NewProc("VkKeyScanW").Call(uintptr(name[0]))
	if uint16(key) == 0xffff {
		return 0
	}
	return key & 255
}

func main() {
	runtime.LockOSThread()
	if len(os.Args) != 3 {
		os.Exit(1)
	}
	mask, err := strconv.Atoi(os.Args[1])
	key := virtualKey(os.Args[2])
	if err != nil || key == 0 {
		os.Exit(1)
	}
	mods := uintptr(0x4000) // MOD_NOREPEAT
	for i, flag := range []uintptr{2, 8, 1, 4} {
		if mask&(1<<i) != 0 {
			mods |= flag
		}
	}
	registered, _, _ := user32.NewProc("RegisterHotKey").Call(0, 1, mods, key)
	if registered == 0 {
		os.Exit(1)
	}
	go func() { io.Copy(io.Discard, os.Stdin); os.Exit(0) }()
	fmt.Println("ready")
	// MSG includes alignment padding around the pointer-sized fields.
	var msg struct {
		window         uintptr
		message        uint32
		wParam, lParam uintptr
		time           uint32
		x, y           int32
		private        uint32
	}
	get := user32.NewProc("GetMessageW")
	state := user32.NewProc("GetAsyncKeyState")
	down := func(key uintptr) bool { v, _, _ := state.Call(key); return v&0x8000 != 0 }
	var timer uintptr
	for {
		ok, _, _ := get.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ok) <= 0 {
			return
		}
		switch msg.message {
		case 0x0312: // WM_HOTKEY
			if timer == 0 {
				timer, _, _ = user32.NewProc("SetTimer").Call(0, 0, 10, 0)
				if timer == 0 {
					return
				}
				fmt.Println("down")
			}
		case 0x0113: // WM_TIMER
			if timer != 0 && (!down(key) || mask&1 != 0 && !down(17) || mask&2 != 0 && !down(91) && !down(92) || mask&4 != 0 && !down(18) || mask&8 != 0 && !down(16)) {
				user32.NewProc("KillTimer").Call(0, timer)
				timer = 0
				fmt.Println("up")
			}
		}
	}
}
