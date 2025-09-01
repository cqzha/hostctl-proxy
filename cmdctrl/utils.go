package cmdctrl

import (
	"errors"
	"os"
	"runtime"
)

func ErrMsg(tp string, cmd string) error {
	var msgText string
	switch tp {
	case "ARN":
		msgText = "Cmd: " + cmd + " is running now"
	case "ASP":
		msgText = "Cmd: " + cmd + " is already stopped"
	default:
		msgText = "Unknown error"
	}
	return errors.New(msgText)
}

func shellPath() string {
	switch runtime.GOOS {
	case "windows":
		return "powershell.exe"
	case "linux":
		return os.Getenv("SHELL")
	case "darwin":
		return os.Getenv("SHELL")
	default:
		return "/system/bin/sh"
	}
}
