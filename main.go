package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// --- WINDOWS BATTERY API ---
var (
	modkernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemPowerStatus = modkernel32.NewProc("GetSystemPowerStatus")
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

	// ACLineStatus: 1 = Charging/Plugged In, 0 = Battery
	charging := ""
	if status.ACLineStatus == 1 {
		charging = "⚡"
	}

	return fmt.Sprintf("Bat: %d%%%s", status.BatteryLifePercent, charging)
}

// --- STOPWATCH LOGIC ---
type Task struct {
	Name     *widget.Entry
	Timer    *widget.Label
	BtnStart *widget.Button
	BtnReset *widget.Button

	StartTime   time.Time
	Accumulated time.Duration // Time stored from previous pauses
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
	t.Timer.SetText(fmt.Sprintf("%02d:%02d:%02d", h, m, s))
}

func (t *Task) Toggle() {
	if t.Running {
		// PAUSE
		t.Accumulated += time.Since(t.StartTime)
		t.Running = false
		t.BtnStart.SetText("▶")
		if t.Ticker != nil {
			t.Ticker.Stop()
			t.StopChan <- true
		}
	} else {
		// START
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
		t.Toggle() // Stop first if running
	}
	t.Accumulated = 0
	t.Timer.SetText("00:00:00")
}

func NewTaskRow() (*Task, *fyne.Container) {
	t := &Task{
		Name:  widget.NewEntry(),
		Timer: widget.NewLabel("00:00:00"),
	}
	t.Name.SetPlaceHolder("Task Name...")

	t.BtnStart = widget.NewButton("▶", func() { t.Toggle() })
	t.BtnReset = widget.NewButton("↺", func() { t.Reset() })

	// Layout: [Play] [Reset] [Name Input (Stretch)] [Timer]
	row := container.New(layout.NewBorderLayout(nil, nil, nil, t.Timer),
		container.NewHBox(t.BtnStart, t.BtnReset),
		t.Timer,
		t.Name, // This goes in the center
	)

	return t, row
}

// --- MAIN APP ---
func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Monitor")

	// 1. Create 3 Task Rows
	_, row1 := NewTaskRow()
	_, row2 := NewTaskRow()
	_, row3 := NewTaskRow()

	// 2. Battery Label (Updates every 5s)
	batLabel := widget.NewLabel("Loading Battery...")
	batLabel.Alignment = fyne.TextAlignCenter
	batLabel.TextStyle = fyne.TextStyle{Bold: true}

	go func() {
		for {
			batLabel.SetText(getWindowsBattery())
			time.Sleep(5 * time.Second)
		}
	}()

	// 3. Layout everything
	content := container.NewVBox(
		widget.NewLabel("Start/Stop | Task Name | Timer"),
		row1,
		row2,
		row3,
		widget.NewSeparator(),
		batLabel,
	)

	myWindow.SetContent(content)

	// --- MAGIC: ALWAYS ON TOP ---
	myWindow.SetFixedSize(true)
	myWindow.Resize(fyne.NewSize(350, 200))

	// This makes it float over other windows
	myWindow.SetAlwaysOnTop(true)

	myWindow.ShowAndRun()
}
