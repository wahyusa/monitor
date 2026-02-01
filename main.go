package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// --- WINDOWS API SETUP (Unchanged) ---
var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	moduser32   = syscall.NewLazyDLL("user32.dll")

	procGetSystemPowerStatus = modkernel32.NewProc("GetSystemPowerStatus")
	procFindWindowW          = moduser32.NewProc("FindWindowW")
	procSetWindowPos         = moduser32.NewProc("SetWindowPos")
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

func setAlwaysOnTop(title string) {
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		titlePtr, _ := syscall.UTF16PtrFromString(title)
		hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))

		if hwnd != 0 {
			hwndTopMost := uintptr(^uintptr(0))
			procSetWindowPos.Call(hwnd, hwndTopMost, 0, 0, 0, 0, 0x0003)
			return
		}
	}
}

// --- STOPWATCH LOGIC ---
type Task struct {
	Name *widget.Entry
	// We replace the direct Label with a Data Binding
	TimeData binding.String

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

	// FIX: Update the data binding, NOT the widget directly
	// This is thread-safe
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
	// Create the thread-safe data binding
	strBinding := binding.NewString()
	strBinding.Set("00:00:00")

	t := &Task{
		Name:     widget.NewEntry(),
		TimeData: strBinding,
	}
	t.Name.SetPlaceHolder("Task Name...")

	t.BtnStart = widget.NewButton("▶", func() { t.Toggle() })
	t.BtnReset = widget.NewButton("↺", func() { t.Reset() })

	// Create label linked to data
	timerLabel := widget.NewLabelWithData(strBinding)
	timerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// --- LAYOUT FIX ---
	// 1. Group buttons together
	buttons := container.NewHBox(t.BtnStart, t.BtnReset)

	// 2. Define Layout: Left=Buttons, Right=Timer
	// Everything else (the Input) goes to the Center
	rowLayout := layout.NewBorderLayout(nil, nil, buttons, timerLabel)

	// 3. Create the container
	// Order matters: Add the border items first, then the center item
	row := container.New(rowLayout,
		buttons,    // Left
		timerLabel, // Right
		t.Name,     // Center (This will stretch)
	)

	return t, row
}

// --- MAIN APP ---
func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Monitor")

	_, row1 := NewTaskRow()
	_, row2 := NewTaskRow()
	_, row3 := NewTaskRow()

	// FIX: Use binding for Battery too
	batBinding := binding.NewString()
	batBinding.Set("Loading Battery...")
	batLabel := widget.NewLabelWithData(batBinding)
	batLabel.Alignment = fyne.TextAlignCenter
	batLabel.TextStyle = fyne.TextStyle{Bold: true}

	// FIX: Update binding in background (Thread Safe)
	go func() {
		for {
			batBinding.Set(getWindowsBattery())
			time.Sleep(5 * time.Second)
		}
	}()

	content := container.NewVBox(
		widget.NewLabel("Start/Stop | Task Name | Timer"),
		row1,
		row2,
		row3,
		widget.NewSeparator(),
		batLabel,
	)

	myWindow.SetContent(content)
	myWindow.SetFixedSize(true)
	myWindow.Resize(fyne.NewSize(350, 200))

	go setAlwaysOnTop("Monitor")

	myWindow.ShowAndRun()
}
