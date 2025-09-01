package main

import (
	"hostctl_proxy/cmdctrl"
	"hostctl_proxy/internal/config"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yusufpapurcu/wmi"
)

type Win32_Process struct {
	Name      string
	ProcessId uint32
}

func SearchProcess(kw string, v interface{}) ([]uint32, error) {
	where := ""
	switch v.(type) {
	case string:
		where = fmt.Sprintf("%s = \"%s\"", kw, v)
		break
	case uint32:
		where = fmt.Sprintf("%s = %d", kw, v)
		break
	default:
		return nil, errors.New("type error of input v")
	}

	var dst []Win32_Process
	q := wmi.CreateQuery(&dst, fmt.Sprintf("WHERE %s", where))
	if err := wmi.Query(q, &dst); err != nil {
		return nil, err
	}

	if len(dst) == 0 {
		return nil, nil
	}

	var sub []uint32
	for _, p := range dst {
		fmt.Printf("process %v", p.ProcessId)
		sub = append(sub, p.ProcessId)
	}

	return sub, nil
}

func ConvertAppConfig(appName string, appCfg *config.AppCfg) (cmdctrl.CommandInfo, error) {
	cmdInfo := cmdctrl.CommandInfo{
		MaxRetries: appCfg.MaxRetries,
		Shell:      appCfg.Shell,
		ArgsFunc: func(args ...string) ([]string, error) {
			var cmdArgs []string
			if appCfg.Socket {
				l, err := sockpManager.GetRamPort()
				if err != nil {
					return nil, err
				}
				defer l.Close()
				port := l.Addr().(*net.TCPAddr).Port
				sockpManager.Register(appName, port)
				cmdArgs = []string{appCfg.Executor, appCfg.RootPath, "--server", "localhost", fmt.Sprintf("%v", port)}
			} else {
				cmdArgs = []string{appCfg.Executor, appCfg.RootPath}
			}

			if len(args) == 0 {
				return append(cmdArgs, appCfg.DefaultArgs...), nil
			} else {
				return append(cmdArgs, args...), nil
			}
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		OnStart: func(ci *cmdctrl.CommandInfo) error {
			logger.AppLog("info", "starting", appName, strings.Join(ci.Args, ", "))
			logger.AppLog("info", "starting", appName, "Start app successfully")
			return nil
		},
		Logentry: logger.GetEntry(logrus.Fields{
			"event": fmt.Sprintf("app %s", appName),
			"topic": "running app",
		}),
	}

	if strings.ToLower(appName) == "omnipeek" {
		cmdInfo.OnStop = func(ci *cmdctrl.CommandInfo) {
			processes, err := SearchProcess("Name", "omnipeek.exe")
			if err != nil {
				logger.AppLog("error", "stopping", appName, fmt.Sprintf("%s failed to execute OnStop, error: %v\n", "omnipeek", err))
				panic(err)
			}

			if len(processes) == 0 {
				logger.AppLog("info", "stopping", appName, "No omnipeek process found")
				return
			}

			for _, pid := range processes {
				if err := CmdKill(uint32(pid), true); err != nil {
					logger.AppLog("error", "stopping", appName, fmt.Sprintf("Failed to kill omnipeek process: %d\n", pid))
					panic(err)
				}
			}
		}
	}
	return cmdInfo, nil
}

// func InitApps() error {
// 	for name, cfg := range serverConfig.List("app") {
// 		appName := name
// 		appCfg := cfg.(*config.AppCfg)
// 		cmdInfo, err := ConvertAppConfig(appName, appCfg)
// 		if err != nil {
// 			logger.AppLog("error", "configuring", appName, err.Error())
// 			return err
// 		}
// 		appManager.Add(appName, cmdInfo)
// 	}
// 	return nil
// }

/* func InitApps(socketPorts *map[string]int, appConfig map[string]*config.AppCfg) error {
	// setup service
	var socketServers []string
	for name, cfg := range appConfig {
		// 默认为随机端口，如果后续为固定端口，需要做更改
		if cfg.Socket {
			socketServers = append(socketServers, name)
		}
	}
	// get ports of socket server
	getSocketServerPorts(socketPorts, socketServers...)
	for name, cfg := range appConfig {
		appName := name
		var cmdArgs []string
		if cfg.Socket {
			cmdArgs = append([]string{cfg.Executor, cfg.RootPath, "--server", "localhost", fmt.Sprintf("%v", (*socketPorts)[name])}, cfg.DefaultArgs...)
		} else {
			cmdArgs = append([]string{cfg.Executor, cfg.RootPath}, cfg.DefaultArgs...)
		}

		var cmdinfo = cmdctrl.CommandInfo{
			MaxRetries: cfg.MaxRetries,
			Shell:      cfg.Shell,
			Args:       cmdArgs,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
			OnStart: func(ci *cmdctrl.CommandInfo) error {
				logger.AppLog("info", "start", appName, strings.Join(ci.Args, ", "))
				logger.AppLog("info", "start", appName, "Start app successfully")
				return nil
			},
		}

		if appName == "omnipeek" {
			cmdinfo.OnStop = func(ci *cmdctrl.CommandInfo) {
				processes, err := searchProcess("Name", "omnipeek.exe")
				if err != nil {
					logger.AppLog("error", "stop", appName, fmt.Sprintf("%s failed to execute OnStop, error: %v\n", "omnipeek", err))
					panic(err)
				}

				if len(processes) == 0 {
					logger.AppLog("info", "stop", appName, "No omnipeek process found")
					return
				}

				for _, pid := range processes {
					if err := CmdKill(uint32(pid), true); err != nil {
						logger.AppLog("error", "stop", appName, fmt.Sprintf("Failed to kill omnipeek process: %d\n", pid))
						panic(err)
					}
				}
			}
		}

		service.Add(appName, cmdinfo)
		logger.AppLog("info", "add", appName, "Adding to auto service successfully")
	}
	return nil
} */
