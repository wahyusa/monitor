# Monitor

Boring stopwatch time runner for personal task tracking and focus management.

Sorry this is yet another AI slop, but I plan to learn more about this and be more confident enough to claim that this project is mine.

Tech stack: Go with Fyne.io for Desktop GUI

No testing, no coverage etc

## Installation

Tested on Windows 11

### Option 1

You can just download `.zip` file from my [GitHub Action artifacts](https://github.com/wahyusa/monitor/actions/runs/21564994683), extract and run the .exe file as usual.

No installation needed.

PS: I am not using GitHub Release for now, but already planned it.
PS: I build for Windows in the cloud using GitHub Action

### Option 2

Build yourself, this project using Go 1.25 and for GitHub Action to works you must set the `go.mod` version to 1.24 because it's the only supported version for the runner.

But if you don't plan to use GitHub Action I belive you can just clone then `go mod tidy` then `go run .` or `go build`

Initial build/run may take some times, tested on my device with celeron processor it takes like 6minutes damn but after that it goes fast.

## Feature

### Keybind

`CTRL+SHIFT+L` Make the GUI non clickable/clickthrough

`CRTL+SHIFT+K` Minimize the GUI to the top center

### General Usage

This app show 3 list of runnable stopwatch for tasks.

I limit this to 3 to make sure I am not rambling randomly and doomscrolling or switching focus everytime.

I need to focus on 1 or 2 things and done it until finished.

You can input task name, then press play button

You can pause or reset stopwatch time

In minimized version, only the first task will be shown

## Personal Review

It works good enough atleast for me...

I can track how long I rambling on stupid feature but endup getting 0 progress

I can force myself to only do 1-2 task at a time

I can also track how long my overall session

This app just take 50-60MB RAM and average 7% CPU usage (trust me bro)

Some people may ask why not just using some existing app or web, the answer is bruhh my device is potato

TODO: Disable github action trigger if I am just editing README.me 
