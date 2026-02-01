package main

import (
	"fmt"
	"image/color"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// --- WINDOWS API SETUP ---
var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	moduser32   = syscall.NewLazyDLL("user32.dll")

	procFindWindowW                = moduser32.NewProc("FindWindowW")
	procSetWindowPos               = moduser32.NewProc("SetWindowPos")
	procGetForegroundWindow        = moduser32.NewProc("GetForegroundWindow")
	procSetLayeredWindowAttributes = moduser32.NewProc("SetLayeredWindowAttributes")
	procSetWindowLongW             = moduser32.NewProc("SetWindowLongW")
	procGetWindowLongW             = moduser32.NewProc("GetWindowLongW")
	procGetAsyncKeyState           = moduser32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics           = moduser32.NewProc("GetSystemMetrics")
)

const (
	GWL_EXSTYLE       = -20
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_TOOLWINDOW  = 0x00000080
	LWA_ALPHA         = 0x00000002

	VK_CONTROL = 0x11
	VK_SHIFT   = 0x10
	VK_L       = 0x4C 
	VK_K       = 0x4B 
	
	SM_CXSCREEN = 0
)

// --- GLOBAL STATE ---
var (
	globalHwnd     uintptr
	isClickThrough = false
	isMiniMode     = false
	
	task1Name binding.String
	task1Time binding.String
	miniText  binding.String 
	
	bgRect    *canvas.Rectangle
)

// --- WINDOWS HELPERS ---
func setWindowAlpha(hwnd uintptr, alpha byte) {
	gwlExStyleInt := int32(GWL_EXSTYLE)
	gwlExStyle := uintptr(uint32(gwlExStyleInt))
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	procSetWindowLongW.Call(hwnd, gwlExStyle, style|uintptr(WS_EX_LAYERED))
	procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), uintptr(LWA_ALPHA))
}

func setClickThrough(hwnd uintptr, enabled bool) {
	gwlExStyleInt := int32(GWL_EXSTYLE)
	gwlExStyle := uintptr(uint32(gwlExStyleInt))
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	
	if enabled {
		procSetWindowLongW.Call(hwnd, gwlExStyle, style|uintptr(WS_EX_TRANSPARENT))
		isClickThrough = true
	} else {
		procSetWindowLongW.Call(hwnd, gwlExStyle, style&^uintptr(WS_EX_TRANSPARENT))
		isClickThrough = false
	}
}

func setToolWindow(hwnd uintptr) {
	gwlExStyleInt := int32(GWL_EXSTYLE)
	gwlExStyle := uintptr(uint32(gwlExStyleInt))
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	procSetWindowLongW.Call(hwnd, gwlExStyle, style|uintptr(WS_EX_TOOLWINDOW))
}

func moveWindowToTopCenter(hwnd uintptr, width, height int) {
	screenWidth, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	x := (int(screenWidth) / 2) - (width / 2)
	y := 10 
	procSetWindowPos.Call(hwnd, uintptr(^uintptr(0)), uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0)
}

func isKeyPressed(vkCode int) bool {
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(vkCode))
	return (ret & 0x8000) != 0
}

func getMyWindowHandle(title string) uintptr {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

// --- STOPWATCH LOGIC ---
type Task struct {
	Name        *widget.Entry
	TimeData    binding.String
	BtnStart    *widget.Button
	BtnReset    *widget.Button
	StartTime   time.Time
	Accumulated time.Duration
	Running     bool
	Ticker      *time.Ticker
	StopChan    chan bool
}

func (t *Task) Toggle() {
	if t.Running {
		t.Accumulated += time.Since(t.StartTime)
		t.Running = false
		t.BtnStart.SetText("▶")
		if t.Ticker != nil { t.Ticker.Stop(); t.StopChan <- true }
	} else {
		t.StartTime = time.Now()
		t.Running = true
		t.BtnStart.SetText("⏸")
		t.Ticker = time.NewTicker(1 * time.Second)
		t.StopChan = make(chan bool)
		go func() {
			for {
				select {
				case <-t.Ticker.C:
					total := t.Accumulated + time.Since(t.StartTime)
					h := int(total.Hours())
					m := int(total.Minutes()) % 60
					s := int(total.Seconds()) % 60
					t.TimeData.Set(fmt.Sprintf("%02d:%02d:%02d", h, m, s))
				case <-t.StopChan:
					return
				}
			}
		}()
	}
}

func (t *Task) Reset() {
	if t.Running { t.Toggle() }
	t.Accumulated = 0
	t.TimeData.Set("00:00:00")
}

func NewTaskRow(isFirst bool) (*Task, *fyne.Container) {
	var strBinding binding.String
	if isFirst {
		task1Time = binding.NewString(); task1Time.Set("00:00:00"); strBinding = task1Time
	} else {
		strBinding = binding.NewString(); strBinding.Set("00:00:00")
	}

	t := &Task{Name: widget.NewEntry(), TimeData: strBinding}
	t.Name.SetPlaceHolder("Task...")
	if isFirst {
		task1Name = binding.NewString(); t.Name.Bind(task1Name)
	}

	t.BtnStart = widget.NewButton("▶", func() { t.Toggle() })
	t.BtnReset = widget.NewButton("↺", func() { t.Reset() })
	t.BtnStart.Importance = widget.LowImportance
	t.BtnReset.Importance = widget.LowImportance

	timerLabel := widget.NewLabelWithData(strBinding)
	timerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	buttons := container.NewHBox(t.BtnStart, t.BtnReset)
	row := container.New(layout.NewBorderLayout(nil, nil, buttons, timerLabel),
		buttons, timerLabel, t.Name)
	return t, row
}

// --- MAIN ---
func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Monitor")
	if drv, ok := myApp.Driver().(desktop.Driver); ok { _ = drv }

	miniText = binding.NewString()
	miniText.Set("Ready")

	// Create UI Rows
	_, row1 := NewTaskRow(true)
	_, row2 := NewTaskRow(false)
	_, row3 := NewTaskRow(false)

	// Normal Mode Content
	normalContent := container.NewVBox(row1, row2, row3)
	
	// Mini Mode Content
	miniLabel := widget.NewLabelWithData(miniText) 
	miniLabel.Alignment = fyne.TextAlignCenter
	miniLabel.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	miniContent := container.NewCenter(miniLabel)

	// Background
	bgRect = canvas.NewRectangle(color.RGBA{20, 20, 20, 240})
	
	// Initial Layout (Normal)
	myWindow.SetContent(container.NewMax(bgRect, normalContent))
	
	// FIX: Use Padded size + fixed width, but don't force extra height
	myWindow.Resize(fyne.NewSize(300, 100)) 
	myWindow.SetFixedSize(true)

	// --- MODE SWITCHER LISTENER ---
	modeSignal := binding.NewBool()
	modeSignal.AddListener(binding.NewDataListener(func() {
		val, _ := modeSignal.Get()
		if val {
			// MINI MODE
			myWindow.SetContent(container.NewMax(bgRect, miniContent))
			myWindow.Resize(fyne.NewSize(450, 40)) 
			if globalHwnd != 0 {
				moveWindowToTopCenter(globalHwnd, 450, 40)
			}
		} else {
			// NORMAL MODE
			myWindow.SetContent(container.NewMax(bgRect, normalContent))
			myWindow.Resize(fyne.NewSize(300, 100)) // Use tight height
		}
	}))

	go func() {
		time.Sleep(1 * time.Second)
		globalHwnd = getMyWindowHandle("Monitor")
		if globalHwnd != 0 {
			setToolWindow(globalHwnd)
			procSetWindowPos.Call(globalHwnd, uintptr(^uintptr(0)), 0, 0, 0, 0, 0x0003)
		}

		var lastCtrlShiftL, lastCtrlShiftK bool

		for {
			time.Sleep(50 * time.Millisecond)

			ctrl := isKeyPressed(VK_CONTROL)
			shift := isKeyPressed(VK_SHIFT)
			pressedL := isKeyPressed(VK_L)
			pressedK := isKeyPressed(VK_K)

			// 1. LOCK
			if ctrl && shift && pressedL && !lastCtrlShiftL {
				if globalHwnd != 0 {
					setClickThrough(globalHwnd, !isClickThrough)
					if isClickThrough {
						bgRect.FillColor = color.RGBA{50, 0, 0, 150}
					} else {
						bgRect.FillColor = color.RGBA{20, 20, 20, 240}
					}
				}
			}
			lastCtrlShiftL = pressedL

			// 2. MINI MODE
			if ctrl && shift && pressedK && !lastCtrlShiftK {
				isMiniMode = !isMiniMode
				modeSignal.Set(isMiniMode)
			}
			lastCtrlShiftK = pressedK

			// 3. TEXT UPDATE
			if isMiniMode {
				name, _ := task1Name.Get()
				if name == "" { name = "Task" }
				timeStr, _ := task1Time.Get()
				dateStr := time.Now().Format("Mon, 02 Jan 2006 15:04")
				miniText.Set(fmt.Sprintf("[ %s %s | %s ]", name, timeStr, dateStr))
			}

			// 4. OPACITY
			if globalHwnd != 0 {
				fg, _, _ := procGetForegroundWindow.Call()
				var alpha byte = 180
				if fg == globalHwnd { alpha = 255 }
				if isClickThrough { alpha = 120 }
				setWindowAlpha(globalHwnd, alpha)
			}
		}
	}()

	// REMOVED: t1.Toggle() so it doesn't auto-start
	myWindow.ShowAndRun()
}