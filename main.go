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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// --- WINDOWS API SETUP ---
var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	moduser32   = syscall.NewLazyDLL("user32.dll")

	procGetSystemPowerStatus       = modkernel32.NewProc("GetSystemPowerStatus")
	procFindWindowW                = moduser32.NewProc("FindWindowW")
	procSetWindowPos               = moduser32.NewProc("SetWindowPos")
	procGetForegroundWindow        = moduser32.NewProc("GetForegroundWindow")
	procSetLayeredWindowAttributes = moduser32.NewProc("SetLayeredWindowAttributes")
	procSetWindowLongW             = moduser32.NewProc("SetWindowLongW")
	procGetWindowLongW             = moduser32.NewProc("GetWindowLongW")
)

// FIX: Define GWL_EXSTYLE as int32 so we can cast it safely later
const (
	GWL_EXSTYLE   = -20
	WS_EX_LAYERED = 0x00080000
	LWA_ALPHA     = 0x00000002
)

type SYSTEM_POWER_STATUS struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func setWindowAlpha(hwnd uintptr, alpha byte) {
	// 1. Get current window style
	// Convert GWL_EXSTYLE properly: use variable to avoid constant overflow
	gwlExStyleInt := int32(GWL_EXSTYLE)
	gwlExStyle := uintptr(uint32(gwlExStyleInt))
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)

	// 2. Add "Layered" flag (Required for transparency)
	procSetWindowLongW.Call(hwnd, gwlExStyle, style|uintptr(WS_EX_LAYERED))

	// 3. Set Alpha (0-255)
	procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), uintptr(LWA_ALPHA))
}

func getWindowsBattery() string {
	var status SYSTEM_POWER_STATUS
	ret, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return "Bat: ??"
	}
	charging := ""
	if status.ACLineStatus == 1 {
		charging = "⚡"
	}
	return fmt.Sprintf("Bat: %d%%%s", status.BatteryLifePercent, charging)
}

func getMyWindowHandle(title string) uintptr {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

func setAlwaysOnTop(title string) {
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		hwnd := getMyWindowHandle(title)
		if hwnd != 0 {
			procSetWindowPos.Call(hwnd, uintptr(^uintptr(0)), 0, 0, 0, 0, 0x0003)
			return
		}
	}
}

// --- STOPWATCH LOGIC (Unchanged) ---
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

func (t *Task) UpdateLabel() {
	total := t.Accumulated
	if t.Running {
		total += time.Since(t.StartTime)
	}
	h := int(total.Hours())
	m := int(total.Minutes()) % 60
	s := int(total.Seconds()) % 60
	t.TimeData.Set(fmt.Sprintf("%02d:%02d:%02d", h, m, s))
}

func (t *Task) Toggle() {
	if t.Running {
		t.Accumulated += time.Since(t.StartTime)
		t.Running = false
		t.BtnStart.SetText("▶")
		if t.Ticker != nil {
			t.Ticker.Stop()
			t.StopChan <- true
		}
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
					t.UpdateLabel()
				case <-t.StopChan:
					return
				}
			}
		}()
	}
}

func (t *Task) Reset() {
	if t.Running {
		t.Toggle()
	}
	t.Accumulated = 0
	t.TimeData.Set("00:00:00")
}

func NewTaskRow() (*Task, *fyne.Container) {
	strBinding := binding.NewString()
	strBinding.Set("00:00:00")
	t := &Task{Name: widget.NewEntry(), TimeData: strBinding}
	t.Name.SetPlaceHolder("Task Name...")
	t.BtnStart = widget.NewButton("▶", func() { t.Toggle() })
	t.BtnReset = widget.NewButton("↺", func() { t.Reset() })

	timerLabel := widget.NewLabelWithData(strBinding)
	timerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	buttons := container.NewHBox(t.BtnStart, t.BtnReset)
	rowLayout := layout.NewBorderLayout(nil, nil, buttons, timerLabel)
	row := container.New(rowLayout, buttons, timerLabel, t.Name)
	return t, row
}

// --- MAIN APP ---
func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Monitor")

	_, row1 := NewTaskRow()
	_, row2 := NewTaskRow()
	_, row3 := NewTaskRow()

	batBinding := binding.NewString()
	batBinding.Set("Loading...")
	batLabel := widget.NewLabelWithData(batBinding)
	batLabel.Alignment = fyne.TextAlignCenter
	batLabel.TextStyle = fyne.TextStyle{Bold: true}

	uiContent := container.NewVBox(
		widget.NewLabel("Start/Stop | Task Name | Timer"),
		row1,
		row2,
		row3,
		widget.NewSeparator(),
		batLabel,
	)

	bgRect := canvas.NewRectangle(color.RGBA{20, 20, 20, 240})
	finalLayout := container.NewMax(bgRect, uiContent)

	myWindow.SetContent(finalLayout)
	myWindow.SetFixedSize(true)
	myWindow.Resize(fyne.NewSize(350, 200))

	// Loops
	go func() {
		for {
			batBinding.Set(getWindowsBattery())
			time.Sleep(5 * time.Second)
		}
	}()

	go setAlwaysOnTop("Monitor")

	// FOCUS DETECTOR (Fixed for correct transparency)
	go func() {
		time.Sleep(1 * time.Second)
		hwnd := getMyWindowHandle("Monitor")

		for {
			time.Sleep(100 * time.Millisecond)
			foregroundHwnd, _, _ := procGetForegroundWindow.Call()

			if foregroundHwnd == hwnd {
				// Focused: Solid (255)
				setWindowAlpha(hwnd, 255)
			} else {
				// Unfocused: Semi-Transparent (180 is roughly 70% opacity)
				// Adjust this value lower (e.g., 100) for more ghost-like effect
				setWindowAlpha(hwnd, 180)
			}
		}
	}()

	myWindow.ShowAndRun()
}