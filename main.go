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

	procGetSystemPowerStatus = modkernel32.NewProc("GetSystemPowerStatus")
	procFindWindowW          = moduser32.NewProc("FindWindowW")
	procSetWindowPos         = moduser32.NewProc("SetWindowPos")
	procGetForegroundWindow  = moduser32.NewProc("GetForegroundWindow") // NEW: Check focus
)

type SYSTEM_POWER_STATUS struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
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

// Returns the HWND (Window Handle) of our app
func getMyWindowHandle(title string) uintptr {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

// Force "Always on Top"
func setAlwaysOnTop(title string) {
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		hwnd := getMyWindowHandle(title)
		if hwnd != 0 {
			// HWND_TOPMOST (-1), SWP_NOMOVE | SWP_NOSIZE (0x0003)
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
	if t.Running { t.Toggle() }
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

	// 1. Setup UI Content
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

	// 2. TRANSPARENCY MAGIC
	// We create a custom background rectangle
	bgRect := canvas.NewRectangle(color.RGBA{20, 20, 20, 240}) // Start Opaque
	
	// We stack the Background BEHIND the UI
	finalLayout := container.NewMax(bgRect, uiContent)

	myWindow.SetContent(finalLayout)
	myWindow.SetFixedSize(true)
	myWindow.Resize(fyne.NewSize(350, 200))
	
	// Important: Tell Fyne the window itself has no background
	myWindow.SetTransparent(true) 

	// 3. Background Loops
	
	// A. Battery Updater
	go func() {
		for {
			batBinding.Set(getWindowsBattery())
			time.Sleep(5 * time.Second)
		}
	}()

	// B. Always on Top enforcer
	go setAlwaysOnTop("Monitor")

	// C. FOCUS DETECTOR (The Acrylic Effect)
	go func() {
		// Colors
		focusedColor := color.RGBA{25, 25, 25, 250}   // Solid-ish Black
		unfocusedColor := color.RGBA{0, 0, 0, 90}     // Transparent Ghost

		for {
			time.Sleep(200 * time.Millisecond)
			
			// Who is the active window right now?
			foregroundHwnd, _, _ := procGetForegroundWindow.Call()
			myHwnd := getMyWindowHandle("Monitor")

			if foregroundHwnd == myHwnd {
				// We are focused! Be Solid.
				bgRect.FillColor = focusedColor
			} else {
				// We lost focus! Be Ghost.
				bgRect.FillColor = unfocusedColor
			}
			// Redraw the background
			bgRect.Refresh()
		}
	}()

	myWindow.ShowAndRun()
}