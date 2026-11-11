package app

import (
	"runtime"
	"time"
)

func isWindowsOS() bool { return runtime.GOOS == "windows" }

func sleepTick() { time.Sleep(20 * time.Millisecond) }
