package main

import (
	//"fmt"

	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"hostctl_proxy/internal/command"
)

type PortsManager struct {
	rl   sync.RWMutex
	pool map[string]int
}

func NewPortsManager() *PortsManager {
	return &PortsManager{
		pool: make(map[string]int),
	}
}

func (manager *PortsManager) Exists(name string) bool {
	manager.rl.RLock()
	defer manager.rl.RUnlock()
	_, ok := manager.pool[name]
	return ok
}

func (manager *PortsManager) Register(name string, port int) error {
	if manager.Exists(name) {
		return fmt.Errorf("%s is already existed", name)
	}
	manager.rl.RLock()
	defer manager.rl.RUnlock()

	manager.pool[name] = port
	return nil
}

func (manager *PortsManager) Deregister(name string) error {
	if !manager.Exists(name) {
		return fmt.Errorf("%s not found", name)
	}

	manager.rl.RLock()
	defer manager.rl.RUnlock()
	delete(manager.pool, name)
	return nil
}

func (manager *PortsManager) GetRamPort() (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", 0))
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (manager *PortsManager) GetSocketPort(name string) (int, error) {
	if !manager.Exists(name) {
		return 0, fmt.Errorf("%s not found", name)
	}

	manager.rl.RLock()
	defer manager.rl.RUnlock()
	return manager.pool[name], nil
}

func CmdKill(pid uint32, force bool) error {
	var c command.Command
	var args []string
	processId := strconv.FormatUint(uint64(pid), 10)
	if runtime.GOOS == "windows" {
		args = append(args, "taskkill")
		if force {
			args = append(args, "/F")
		}
		args = append(args, "/PID", processId)
	} else {
		args = append(args, "kill", "-9", processId)
	}
	c.Args = args

	if err := c.Run(); err != nil {
		return err
	}

	return nil
}

/* func dynamicStruct(raw []*config.PutbodyCfg) reflect.Type {
	var fields []reflect.StructField
	for _, v := range raw {
		field := reflect.StructField{
			Name: v.Name,
			Tag:  reflect.StructTag(v.Tag),
			Type: reflect.TypeOf(v.Default),
		}

		fields = append(fields, field)
	}

	tp := reflect.StructOf(fields)
	return tp
} */

func convPutbody() {

}

func Gbk2Utf8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewDecoder())
	d, e := io.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func GetLogFile() (*os.File, error) {
	curr_time := time.Now().Local().Format("2006-01-02_15-04-05")
	tmp := strings.Split(curr_time, "_")
	err := os.MkdirAll("./log/hostctl_proxy/"+tmp[0], 0644)
	if err != nil {
		return nil, err
	}

	fileName := fmt.Sprintf("./log/hostctl_proxy/%s/%s.log", tmp[0], tmp[1])
	logFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return logFile, nil
}

func GetSocketUrl(name string) (string, error) {
	port, err := sockpManager.GetSocketPort(name)
	if err != nil {
		return "", err
	}
	socketUrl := fmt.Sprintf("localhost:%v", port)
	return socketUrl, nil
}

func AppoutJsonSerialize(data interface{}) (interface{}, error) {
	s, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("%v is not json serializable", data)
	}
	var js interface{}
	err := json.Unmarshal([]byte(s), &js)
	if err != nil {
		return nil, err
	}
	return js, nil
}
