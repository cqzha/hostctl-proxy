package cmdctrl

import (
	"fmt"
	"io"

	//"log"

	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type CommandInfo struct {
	Environ         []string
	Args            []string
	ArgsFunc        func(args ...string) ([]string, error)
	MaxRetries      int
	NextLaunchWait  time.Duration
	RecoverDuration time.Duration
	StopSignal      os.Signal
	Shell           bool

	OnStart func(*CommandInfo) error
	OnStop  func(*CommandInfo)

	Stderr io.Writer
	Stdout io.Writer
	Stdin  io.Reader

	Logentry *logrus.Entry
}

type ProcessKeeper struct {
	name       string
	mu         sync.Mutex
	cmdInfo    CommandInfo
	cmd        *exec.Cmd
	retries    int
	running    bool
	keeping    bool
	stopC      chan bool
	runBeganAt time.Time
	donewg     *sync.WaitGroup
}

type CommandCtrl struct {
	rl   sync.RWMutex
	cmds map[string]*ProcessKeeper
}

// type CommandPipe struct {
// 	Inr  *os.File
// 	Inw  *os.File
// 	Outr *os.File
// 	Outw *os.File
// }

// type CommandPipeCtrl struct {
// 	rl    sync.RWMutex
// 	pipes map[string]*CommandPipe
// }

func New(amount int) *CommandCtrl {
	return &CommandCtrl{
		cmds: make(map[string]*ProcessKeeper, amount),
	}
}

func (cc *CommandCtrl) List() []string {
	cc.rl.RLock()
	defer cc.rl.RUnlock()
	keys := make([]string, len(cc.cmds))
	i := 0
	for k, _ := range cc.cmds {
		keys[i] = k
		i++
	}
	return keys
}

func (cc *CommandCtrl) Exists(name string) bool {
	cc.rl.RLock()
	defer cc.rl.RUnlock()
	_, ok := cc.cmds[name]
	return ok
}

func (cc *CommandCtrl) Add(name string, c CommandInfo) error {
	if len(c.Args) == 0 && c.ArgsFunc == nil {
		return fmt.Errorf("not enough length of Args, must larger than 1")
	}

	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.RecoverDuration == 0 {
		c.RecoverDuration = 20 * time.Second
	}
	if c.NextLaunchWait == 0 {
		c.NextLaunchWait = 500 * time.Millisecond
	}
	if c.StopSignal == nil {
		c.StopSignal = syscall.SIGTERM
	}

	cc.rl.RLock()
	defer cc.rl.RUnlock()

	if _, exists := cc.cmds[name]; exists {
		return fmt.Errorf("app name conflict: %s", name)
	}
	cc.cmds[name] = &ProcessKeeper{
		name:    name,
		cmdInfo: c,
	}
	return nil
}

func (cc *CommandCtrl) Remove(name string) error {
	cc.rl.RLock()
	defer cc.rl.RUnlock()

	pkeeper, ok := cc.cmds[name]
	if !ok {
		return fmt.Errorf("app not found: %s ", name)
	}

	if pkeeper.keeping {
		return fmt.Errorf("app %s is running, stop first", name)
	}

	delete(cc.cmds, name)
	return nil
}

func (cc *CommandCtrl) Start(name string, args ...string) error {
	cc.rl.RLock()
	defer cc.rl.RUnlock()

	pkeeper, ok := cc.cmds[name]
	if !ok {
		return fmt.Errorf("app not found: %s", name)
	}

	// fmt.Printf("%v args %v\n", name, pkeeper.cmdInfo.Args)
	if pkeeper.cmdInfo.OnStart != nil {
		if err := pkeeper.cmdInfo.OnStart(&pkeeper.cmdInfo); err != nil {
			return err
		}
	}

	ch := pkeeper.start(args...)
	select {
	case err := <-ch:
		return err
	case <-time.After(time.Second * time.Duration(pkeeper.cmdInfo.MaxRetries+3)):
		return nil
	}
}

func (cc *CommandCtrl) Stop(name string, waits ...bool) error {
	cc.rl.RLock()
	defer cc.rl.RUnlock()

	pkeeper, ok := cc.cmds[name]
	if !ok {
		return fmt.Errorf("app not found: %s", name)
	}
	wait := false
	if len(waits) > 0 {
		wait = waits[0]
	}
	return pkeeper.stop(wait)
}

func (cc *CommandCtrl) StopAll() {
	for _, pkeeper := range cc.cmds {
		pkeeper.stop(true)
	}
}

func (cc *CommandCtrl) Restart(name string) error {
	cc.Stop(name, true)
	return cc.Start(name)
}

func (cc *CommandCtrl) UpdateArgs(name string, args ...string) error {
	cc.rl.RLock()
	defer cc.rl.RUnlock()

	if len(args) <= 0 {
		return fmt.Errorf("not enough length of args, must larger than 1")
	}
	pkeeper, ok := cc.cmds[name]
	if !ok {
		return fmt.Errorf("app not found: %s", name)
	}
	pkeeper.cmdInfo.Args = args
	if !pkeeper.keeping {
		return nil
	}
	return cc.Restart(name)
}

func (cc *CommandCtrl) Running(name string) bool {
	cc.rl.RLock()
	defer cc.rl.RUnlock()
	pkeeper, ok := cc.cmds[name]
	if !ok {
		return false
	}
	return pkeeper.keeping
}

// func (cc *CommandCtrl) Communicate(name string, input []byte) ([]byte, error) {
// 	cc.rl.RLock()
// 	defer cc.rl.RUnlock()
// 	pkeeper, ok := cc.cmds[name]
// 	if !ok {
// 		return nil, errors.New("app not found: " + name)
// 	}

// 	stdinPipe, err := pkeeper.cmd.StdinPipe()
// 	if err != nil {
// 		return nil, errors.New(fmt.Sprintf("Cmd [%s] create stdin pipe error: %#v", name, err))
// 	}

// 	stdoutPipe, err := pkeeper.cmd.StdoutPipe()
// 	defer stdoutPipe.Close()
// 	if err != nil {
// 		return nil, errors.New(fmt.Sprintf("Cmd [%s] create stdout pipe error: %#v", name, err))
// 	}

// 	go func() {
// 		defer stdinPipe.Close()
// 		stdinPipe.Write(input)
// 	}()

// 	out, err := ioutil.ReadAll(stdoutPipe)
// 	if err != nil {
// 		return nil, errors.New(fmt.Sprintf("Cmd [%s] read stdout error: %#v", name, err))
// 	}

// 	return out, nil
// }

func goFunc(f func() error) chan error {
	errC := make(chan error, 1)
	go func() {
		errC <- f()
	}()
	return errC
}

func (p *ProcessKeeper) start(args ...string) chan error {
	chErr := make(chan error, 1)
	var startErr error
	p.cmdInfo.Logentry.Infof("[%s] is starting\n, retries %d\n", p.name, p.cmdInfo.MaxRetries)
	// fmt.Printf("[%s] is starting\n, retries %d\n", p.name, p.cmdInfo.MaxRetries)
	p.mu.Lock()
	if p.keeping {
		p.mu.Unlock()
		p.cmdInfo.Logentry.Errorf("[%s] is running\n", p.name)
		// return ErrMsg("ARN", p.name)
	}
	p.keeping = true
	p.stopC = make(chan bool, 1)
	p.retries = 0
	p.donewg = &sync.WaitGroup{}
	p.donewg.Add(1)
	p.mu.Unlock()

	go func() {
		for {
			if p.retries < 0 {
				p.retries = 0
			}
			if p.retries > p.cmdInfo.MaxRetries {
				chErr <- startErr
				break
			}
			cmdArgs := p.cmdInfo.Args
			if p.cmdInfo.ArgsFunc != nil {
				var er error
				cmdArgs, er = p.cmdInfo.ArgsFunc(args...)
				if er != nil {
					//fmt.Printf("ArgsFunc error: %v\n", er)
					p.cmdInfo.Logentry.Errorf("ArgsFunc error: %v\n", er)
					chErr <- er
					goto CMD_DONE
				}
			}
			// fmt.Printf("[%s] Args: %v\n", p.name, cmdArgs)
			p.cmdInfo.Logentry.Infof("[%s] Args: %v\n", p.name, cmdArgs)
			if p.cmdInfo.Shell {
				cmdArgs = []string{shellPath(), "-c", strings.Join(cmdArgs, " ")}
			}
			p.cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
			p.cmd.Env = append(os.Environ(), p.cmdInfo.Environ...)
			p.cmd.Stdin = p.cmdInfo.Stdin
			p.cmd.Stdout = p.cmdInfo.Stdout
			p.cmd.Stderr = p.cmdInfo.Stderr
			// fmt.Printf("[%s] args: %v, env: %v\n", p.name, cmdArgs, p.cmdInfo.Environ)
			p.cmdInfo.Logentry.Infof("[%s] args: %v, env: %v\n", p.name, cmdArgs, p.cmdInfo.Environ)
			if err := p.cmd.Start(); err != nil {
				p.cmdInfo.Logentry.Errorf("[%s] app start err: %v\n", p.name, err)
				chErr <- err
				goto CMD_DONE
			}
			// fmt.Printf("[%s] program pid: %d\n", p.name, p.cmd.Process.Pid)
			p.cmdInfo.Logentry.Infof("[%s] program pid: %d\n", p.name, p.cmd.Process.Pid)
			p.runBeganAt = time.Now()
			p.running = true
			cmdC := goFunc(p.cmd.Wait)
			// fmt.Printf("cmdC is %v\n", cmdC)
			p.cmdInfo.Logentry.Infof("[%s] cmdC is %v\n", p.name, cmdC)
			select {
			case cmdErr := <-cmdC:
				if cmdErr != nil {
					// fmt.Printf("[%s] cmd wait err: %v\n", p.name, cmdErr)
					p.cmdInfo.Logentry.Errorf("[%s] cmd wait err: %v\n", p.name, cmdErr)
					startErr = cmdErr
				}
				if time.Since(p.runBeganAt) > p.cmdInfo.RecoverDuration {
					p.retries -= 2
				}
				p.retries++
				goto CMD_IDLE
			case <-p.stopC:
				p.terminate(cmdC)
				goto CMD_DONE
			}
		CMD_IDLE:
			// fmt.Printf("[%s] idle for %v\n", p.name, p.cmdInfo.NextLaunchWait)
			p.cmdInfo.Logentry.Infof("[%s] idle for %v\n", p.name, p.cmdInfo.NextLaunchWait)
			p.running = false
			select {
			case <-p.stopC:
				goto CMD_DONE
			case <-time.After(p.cmdInfo.NextLaunchWait):
				// do nothing
			}
		}
	CMD_DONE:
		// fmt.Printf("[%s] program finished\n", p.name)
		p.cmdInfo.Logentry.Infof("[%s] program finished\n", p.name)
		if p.cmdInfo.OnStop != nil {
			p.cmdInfo.OnStop(&p.cmdInfo)
		}
		p.mu.Lock()
		p.running = false
		p.keeping = false
		p.donewg.Done()
		p.mu.Unlock()
	}()
	return chErr
}

func (p *ProcessKeeper) terminate(cmdC chan error) {
	if runtime.GOOS == "windows" {
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
		return
	}
	if p.cmd.Process != nil {
		p.cmd.Process.Signal(p.cmdInfo.StopSignal)
	}
	terminateWait := 3 * time.Second
	select {
	case <-cmdC:
		break
	case <-time.After(terminateWait):
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
	}
	return
}

func (p *ProcessKeeper) stop(wait bool) error {
	p.mu.Lock()
	if !p.keeping {
		p.mu.Unlock()
		p.cmdInfo.Logentry.Errorf("[%s] is already stopped", p.name)
		return fmt.Errorf("[%s] is already stopped", p.name)
	}
	select {
	case p.stopC <- true:
	default:
	}
	donewg := p.donewg
	p.mu.Unlock()
	if wait {
		donewg.Wait()
	}
	return nil
}

// func NewPipeCtrl(amount int) *CommandPipeCtrl {
// 	return &CommandPipeCtrl{
// 		pipes: make(map[string]*CommandPipe, amount),
// 	}
// }

// func (pc *CommandPipeCtrl) Exists(name string) bool {
// 	pc.rl.RLock()
// 	defer pc.rl.RUnlock()
// 	_, ok := pc.pipes[name]
// 	return ok
// }

// func (pc *CommandPipeCtrl) Add(name string, cmdPipe *CommandPipe) error {
// 	pc.rl.RLock()
// 	defer pc.rl.RUnlock()
// 	if pc.Exists(name) {
// 		return fmt.Errorf("Pipe name conflict: " + name)
// 	}

// 	pc.pipes[name] = cmdPipe
// 	return nil
// }

// func (pc *CommandPipeCtrl) FeedIn(name string, data []byte) error {
// 	pc.rl.RLock()
// 	defer pc.rl.RUnlock()

// 	if !pc.Exists(name) {
// 		return fmt.Errorf(fmt.Sprintf("Pipe [%s] does not exist", name))
// 	}

// 	fmt.Printf("feed in %s", data)
// 	if _, err := pc.pipes[name].Inw.Write(data); err != nil {
// 		return err
// 	}
// 	return nil
// }

// func (pc *CommandPipeCtrl) ReadOut(name string) ([]byte, error) {
// 	pc.rl.RLock()
// 	defer pc.rl.RUnlock()

// 	if !pc.Exists(name) {
// 		return nil, fmt.Errorf(fmt.Sprintf("Pipe [%s] does not exist", name))
// 	}

// 	fmt.Printf("reading")
// 	data, err := ioutil.ReadAll(pc.pipes[name].Outr)
// 	if err != nil {
// 		return nil, err
// 	}

// 	fmt.Printf("read success")
// 	return data, err
// }

// func (pc *CommandPipeCtrl) Delete(name string) error {
// 	pc.rl.RLock()
// 	defer pc.rl.RUnlock()
// 	if !pc.Exists(name) {
// 		return fmt.Errorf(fmt.Sprintf("Pipe [%s] does not exist", name))
// 	}

// 	pipe := pc.pipes[name]
// 	if err := pipe.Inr.Close(); err != nil {
// 		return err
// 	}
// 	if err := pipe.Inw.Close(); err != nil {
// 		return err
// 	}
// 	if err := pipe.Outr.Close(); err != nil {
// 		return err
// 	}
// 	if err := pipe.Outw.Close(); err != nil {
// 		return err
// 	}

// 	delete(pc.pipes, name)
// 	return nil
// }
