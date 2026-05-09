package utils

import (
	"fmt"

	golog "github.com/ipfs/go-log/v2"
)

type Color string

const (
	Black   Color = "\u001b[30m"
	Red     Color = "\u001b[31m"
	Green   Color = "\u001b[32m"
	Yellow  Color = "\u001b[33m"
	Blue    Color = "\u001b[34m"
	Magenta Color = "\u001b[35m"
	Cyan    Color = "\u001b[36m"
	White   Color = "\u001b[37m"
	Reset   Color = "\u001b[0m"
)

func ColorizePrint(f string, args ...interface{}) {
	fm := string(White) + f + string(Reset)
	fmt.Printf(fm, args...)
}

func ColorizeInfo(logger golog.EventLogger, f string, args ...interface{}) {
	fm := string(Green) + f + string(Reset)
	logger.Infof(fm, args...)
}

func ColorizeDebug(logger golog.EventLogger, f string, args ...interface{}) {
	fm := string(Yellow) + f + string(Reset)
	logger.Debugf(fm, args...)
}

func ColorizeError(logger golog.EventLogger, f string, args ...interface{}) {
	fm := string(Red) + f + string(Reset)
	logger.Errorf(fm, args...)
}

func ColorizeWarn(logger golog.EventLogger, f string, args ...interface{}) {
	fm := string(Cyan) + f + string(Reset)
	logger.Debugf(fm, args...)
}
