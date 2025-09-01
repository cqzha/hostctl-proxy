package main

import (
	"hostctl_proxy/cmdctrl"
	"hostctl_proxy/internal/config"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

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

	return cmdInfo, nil
}

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

		service.Add(appName, cmdinfo)
		logger.AppLog("info", "add", appName, "Adding to auto service successfully")
	}
	return nil
}
*/
