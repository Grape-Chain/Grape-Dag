package run

import (
	"fmt"
	"os"
	"syscall"
)

func postProcessId(fn string) {
	fname := fmt.Sprintf("./%s.pid", fn)
	fdesc, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logger.Errorf("%s", err.Error())
		return
	}
	_, err = fdesc.WriteString(fmt.Sprintf("%d", syscall.Getpid()))
	if err != nil {
		logger.Errorf("%s", err.Error())
	}
	fdesc.Close()
}

func cleanProcessId(fn string) {
	fname := fmt.Sprintf("%s.pid", fn)
	os.Remove(fname)
}
