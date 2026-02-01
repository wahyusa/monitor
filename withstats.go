package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
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
	procGetAsyncKeyState           = moduser32.NewProc("GetAsyncKeyState")
)

const (
	GWL_STYLE     = -16
	GWL_EXSTYLE   = -20
	WS_EX_LAYERED = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	LWA_ALPHA     = 0x00000002
	
	VK_CONTROL = 0x11
	VK_SHIFT   = 0x10
	VK_L       = 0x4C
)

type SYSTEM_POWER_STATUS struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// --- DATA STRUCTURES ---
type TaskLog struct {
	TaskName  string    `json:"task_name"`
	Duration  int64     `json:"duration_seconds"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Date      string    `json:"date"`
}

type DailyStats struct {
	Date          string
	TotalTime     int64
	TaskBreakdown map[string]int64
	Sessions      int
}

// --- GLOBAL STATE ---
var (
	isClickThrough = false
	lockModeLabel  *widget.Label
)

// --- LOGGING ---
func getLogFilePath() string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".monitor_logs")
	os.MkdirAll(logDir, 0755)
	return filepath.Join(logDir, "task_logs.jsonl")
}

func saveTaskLog(log TaskLog) error {
	f, err := os.OpenFile(getLogFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(log)
	if err != nil {
		return err
	}

	_, err = f.WriteString(string(data) + "\n")
	return err
}

func flushLogs() error {
	return os.Remove(getLogFilePath())
}

func loadAllLogs() ([]TaskLog, error) {
	data, err := os.ReadFile(getLogFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskLog{}, nil
		}
		return nil, err
	}

	var logs []TaskLog
	lines := string(data)
	for _, line := range splitLines(lines) {
		if line == "" {
			continue
		}
		var log TaskLog
		if err := json.Unmarshal([]byte(line), &log); err == nil {
			logs = append(logs, log)
		}
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].StartTime.After(logs[j].StartTime)
	})

	return logs, nil
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, ch := range s {
		if ch == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func calculateDailyStats(logs []TaskLog) map[string]DailyStats {
	stats := make(map[string]DailyStats)

	for _, log := range logs {
		date := log.Date
		stat, exists := stats[date]
		if !exists {
			stat = DailyStats{
				Date:          date,
				TaskBreakdown: make(map[string]int64),
			}
		}

		stat.TotalTime += log.Duration
		stat.TaskBreakdown[log.TaskName] += log.Duration
		stat.Sessions++
		stats[date] = stat
	}

	return stats
}

// --- HELPER FUNCTIONS ---
func setWindowAlpha(hwnd uintptr, alpha byte) {
	gwlExStyleInt := int32(GWL_EXSTYLE)
	gwlExStyle := uintptr(uint32(gwlExStyleInt))
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	procSetWindowLongW.Call(hwnd, gwlExStyle, style|uintptr(WS_EX_LAYERED))
	procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), uintptr(LWA_ALPHA))
}

// Toggle click-through mode
func setClickThrough(hwnd uintptr, enabled bool) {
	gwlExStyleInt := int32(GWL_EXSTYLE)
	gwlExStyle := uintptr(uint32(gwlExStyleInt))
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	
	if enabled {
		// Enable click-through
		procSetWindowLongW.Call(hwnd, gwlExStyle, style|uintptr(WS_EX_TRANSPARENT))
		isClickThrough = true
		if lockModeLabel != nil {
			lockModeLabel.SetText("🔒 LOCKED (Ctrl+Shift+L to unlock)")
		}
	} else {
		// Disable click-through
		procSetWindowLongW.Call(hwnd, gwlExStyle, style&^uintptr(WS_EX_TRANSPARENT))
		isClickThrough = false
		if lockModeLabel != nil {
			lockModeLabel.SetText("")
		}
	}
}

// Check if key is pressed
func isKeyPressed(vkCode int) bool {
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(vkCode))
	return (ret & 0x8000) != 0
}

func getWindowsBattery() string {
	var status SYSTEM_POWER_STATUS
	ret, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return "BAT:??"
	}

	charging := ""
	if status.ACLineStatus == 1 {
		charging = "⚡"
	}

	return fmt.Sprintf("%d%%%s", status.BatteryLifePercent, charging)
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

func formatDuration(seconds int64) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
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
		elapsed := time.Since(t.StartTime)
		t.Accumulated += elapsed
		t.Running = false
		t.BtnStart.SetText("▶")

		if t.Ticker != nil {
			t.Ticker.Stop()
			t.StopChan <- true
		}

		taskName := t.Name.Text
		if taskName == "" {
			taskName = "Unnamed"
		}

		totalSeconds := int64(t.Accumulated.Seconds())
		if totalSeconds > 0 {
			log := TaskLog{
				TaskName:  taskName,
				Duration:  totalSeconds,
				StartTime: t.StartTime,
				EndTime:   time.Now(),
				Date:      time.Now().Format("2006-01-02"),
			}
			saveTaskLog(log)
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
	t.Name.SetPlaceHolder("Task...")
	t.BtnStart = widget.NewButton("▶", func() { t.Toggle() })
	t.BtnReset = widget.NewButton("↺", func() { t.Reset() })

	timerLabel := widget.NewLabelWithData(strBinding)
	timerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	buttons := container.NewHBox(t.BtnStart, t.BtnReset)
	rowLayout := layout.NewBorderLayout(nil, nil, buttons, timerLabel)
	row := container.New(rowLayout, buttons, timerLabel, t.Name)
	return t, row
}

// --- STATS TAB ---
func createStatsTab(myWindow fyne.Window) *fyne.Container {
	statsText := widget.NewLabel("Loading...")
	statsText.TextStyle = fyne.TextStyle{Monospace: true}

	refreshStats := func() {
		logs, err := loadAllLogs()
		if err != nil {
			statsText.SetText("Error loading logs")
			return
		}

		if len(logs) == 0 {
			statsText.SetText("No data yet\n\nStart tracking to see stats")
			return
		}

		stats := calculateDailyStats(logs)

		var dates []string
		for date := range stats {
			dates = append(dates, date)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dates)))

		text := fmt.Sprintf("Total Sessions: %d\n\n", len(logs))

		displayCount := 7
		if len(dates) < displayCount {
			displayCount = len(dates)
		}

		for i := 0; i < displayCount; i++ {
			date := dates[i]
			stat := stats[date]

			text += fmt.Sprintf("%s\n", date)
			text += fmt.Sprintf("Total: %s (%d sessions)\n", formatDuration(stat.TotalTime), stat.Sessions)

			type taskTime struct {
				name string
				time int64
			}
			var tasks []taskTime
			for name, dur := range stat.TaskBreakdown {
				tasks = append(tasks, taskTime{name, dur})
			}
			sort.Slice(tasks, func(i, j int) bool {
				return tasks[i].time > tasks[j].time
			})

			for _, task := range tasks {
				text += fmt.Sprintf("  %s: %s\n", task.name, formatDuration(task.time))
			}
			text += "\n"
		}

		statsText.SetText(text)
	}

	refreshBtn := widget.NewButton("Refresh", refreshStats)
	
	flushBtn := widget.NewButton("Flush All Stats", func() {
		dialog.ShowConfirm("Delete All Stats?", 
			"This will permanently delete all logged data.\n\nAre you sure?",
			func(confirmed bool) {
				if confirmed {
					err := flushLogs()
					if err != nil {
						dialog.ShowError(err, myWindow)
					} else {
						statsText.SetText("All stats deleted")
					}
				}
			}, myWindow)
	})
	flushBtn.Importance = widget.DangerImportance

	go refreshStats()

	scroll := container.NewScroll(statsText)
	scroll.SetMinSize(fyne.NewSize(280, 110))

	buttons := container.NewHBox(refreshBtn, flushBtn)
	return container.NewBorder(nil, buttons, nil, nil, scroll)
}

// --- MAIN APP ---
func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Monitor")

	_, row1 := NewTaskRow()
	_, row2 := NewTaskRow()
	_, row3 := NewTaskRow()

	statusBinding := binding.NewString()
	statusBinding.Set("Loading...")

	statusLabel := widget.NewLabelWithData(statusBinding)
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// Lock mode indicator
	lockModeLabel = widget.NewLabel("")
	lockModeLabel.Alignment = fyne.TextAlignCenter
	lockModeLabel.TextStyle = fyne.TextStyle{Bold: true}

	headerLabel := widget.NewLabel("▶/⏸ | Task | Timer")
	headerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	tasksTab := container.NewVBox(
		headerLabel,
		row1,
		row2,
		row3,
		widget.NewSeparator(),
		statusLabel,
		lockModeLabel,
	)

	statsTab := createStatsTab(myWindow)

	tabs := container.NewAppTabs(
		container.NewTabItem("Tasks", tasksTab),
		container.NewTabItem("Stats", statsTab),
	)

	bgRect := canvas.NewRectangle(color.RGBA{18, 18, 18, 245})
	finalLayout := container.NewMax(bgRect, tabs)

	myWindow.SetContent(finalLayout)
	myWindow.SetFixedSize(true)
	myWindow.Resize(fyne.NewSize(300, 220))

	// Update status bar
	go func() {
		for {
			bat := getWindowsBattery()
			now := time.Now()
			datetime := now.Format("02 Jan 15:04")

			statusBinding.Set(fmt.Sprintf("%s | %s", bat, datetime))

			time.Sleep(1 * time.Second)
		}
	}()

	go setAlwaysOnTop("Monitor")

	// Keyboard shortcut listener and focus-based opacity
	go func() {
		time.Sleep(1 * time.Second)
		hwnd := getMyWindowHandle("Monitor")
		
		var lastCtrlShiftL bool

		for {
			time.Sleep(50 * time.Millisecond)
			
			// Check for Ctrl+Shift+L
			ctrlShiftL := isKeyPressed(VK_CONTROL) && isKeyPressed(VK_SHIFT) && isKeyPressed(VK_L)
			
			if ctrlShiftL && !lastCtrlShiftL {
				// Toggle click-through mode
				setClickThrough(hwnd, !isClickThrough)
			}
			lastCtrlShiftL = ctrlShiftL
			
			// Focus-based opacity (only when NOT in click-through mode)
			if !isClickThrough {
				foregroundHwnd, _, _ := procGetForegroundWindow.Call()
				if foregroundHwnd == hwnd {
					setWindowAlpha(hwnd, 255)
				} else {
					setWindowAlpha(hwnd, 180)
				}
			} else {
				// In click-through mode, make it more transparent
				setWindowAlpha(hwnd, 150)
			}
		}
	}()

	myWindow.ShowAndRun()
}