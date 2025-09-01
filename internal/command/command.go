package command

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func cmdError2Code(err error) int {
	if err == nil {
		return 0
	}
	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0

		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	return 128
}

type Command struct {
	Args    []string
	Timeout time.Duration
	Shell   bool
	Stdout  io.Writer
	Stderr  io.Writer
}

func (c *Command) shellPath() string {
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

func (c *Command) computedArgs() (name string, args []string) {
	if c.Shell {
		cmdline := strings.Join(c.Args, " ")
		args = append(args, "-c", cmdline)
		//fmt.Printf("args %#v", args)
		return c.shellPath(), args
	}

	return c.Args[0], c.Args[1:]
}

func (c Command) newCommand() *exec.Cmd {
	name, args := c.computedArgs()
	cmd := exec.Command(name, args...)
	if c.Stdout != nil {
		cmd.Stdout = c.Stdout
	}
	if c.Stderr != nil {
		cmd.Stderr = c.Stderr
	}
	return cmd
}

func (c Command) Run() error {
	cmd := c.newCommand()
	if c.Timeout > 0 {
		timer := time.AfterFunc(c.Timeout, func() {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		})
		defer timer.Stop()
	}
	return cmd.Run()
}

func (c Command) StartBackground() (pid int, err error) {
	cmd := c.newCommand()
	err = cmd.Start()
	if err != nil {
		return
	}
	pid = cmd.Process.Pid
	return
}

func (c Command) Output() (output []byte, err error) {
	var b bytes.Buffer
	c.Stdout = &b
	c.Stderr = nil
	err = c.Run()
	return b.Bytes(), err
}

func (c Command) CombinedOutput() (output []byte, err error) {
	var b bytes.Buffer
	c.Stdout = &b
	c.Stderr = &b
	err = c.Run()
	return b.Bytes(), err
}

func (c Command) CombinedOutputString() (output string, err error) {
	bytesOutput, err := c.CombinedOutput()
	return string(bytesOutput), err
}

func runShell(args ...string) (output []byte, err error) {
	return Command{
		Args:    args,
		Shell:   true,
		Timeout: 10 * time.Minute,
	}.CombinedOutput()
}

func runShellOutput(args ...string) (output []byte, err error) {
	return Command{
		Args:    args,
		Shell:   true,
		Timeout: 10 * time.Minute,
	}.Output()
}

func runShellTimeout(duration time.Duration, args ...string) (output []byte, err error) {
	return Command{
		Args:    args,
		Shell:   true,
		Timeout: duration,
	}.CombinedOutput()
}

func RunShell(args []string, c chan string) {
	var (
		cmd *exec.Cmd
		out strings.Builder
	)

	sysname := runtime.GOOS
	if sysname == "windows" {
		cmd = exec.Command("cmd", args...)
	} else {
		cmd = exec.Command(args[0], args[1:]...)
	}
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		panic(err)
	}

	c <- out.String() // transfer standard out to channel
}
