package main

import (
	"hostctl_proxy/cmdctrl"
	"hostctl_proxy/internal/config"
	"hostctl_proxy/internal/logutils"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gorilla/websocket"
)

const (
	version = "0.0.1"
)

var (
	//verFlag = app.Flag("version", "Show version").Bool()
	appManager   *cmdctrl.CommandCtrl
	jsonPath     string
	serverConfig = config.New()
	upgrader     = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	logger       = &logutils.ServerLogger{}
	wsManager    = NewWebsocketManager()
	sockpManager = NewPortsManager()
)

func NewServer() *Server {
	server := &Server{}
	server.initHttpServer()
	return server
}

func InitApps() error {
	for name, cfg := range serverConfig.List("app") {
		appName := name
		appCfg := cfg.(*config.AppCfg)
		cmdInfo, err := ConvertAppConfig(appName, appCfg)
		if err != nil {
			logger.AppLog("error", "configuring", appName, err.Error())
			return err
		}
		if err := appManager.Add(appName, cmdInfo); err != nil {
			logger.AppLog("error", "adding", appName, err.Error())
			return err
		} else {
			logger.AppLog("info", "adding", appName, fmt.Sprintf("%s is ready", appName))
		}
	}
	return nil
}

func main() {

	// 变更一下命令行参数
	var (
		serverCmd  = kingpin.Command("server", "Start service")
		serverHost = serverCmd.Flag("host", "Service address, default 127.0.0.1").Default("127.0.0.1").IP()
		serverPort = serverCmd.Flag("port", "Service port, default 8080").Default("8080").Int()
		lAddr      = ""
		close      = make(chan os.Signal, 1)
	)
	signal.Notify(close, os.Interrupt)
	// command line
	kingpin.Version(version)
	kingpin.HelpFlag.Short('h')

	switch kingpin.Parse() {
	case serverCmd.FullCommand():
		// do nothing
	}

	// set up server log
	logFile, err := GetLogFile()
	if err != nil {
		panic(err)
	}
	defer logFile.Close()
	logger.Init(logFile)

	// load initial configuration
	dir, _ := os.Getwd()
	jsonPath = dir + string(os.PathSeparator) + "config" + string(os.PathSeparator) + "config.json"
	if err := serverConfig.Init(jsonPath); err != nil {
		logger.ConfigLog("error", "initiating server config", err.Error())
		panic(err)
	}

	// init app manager service
	amount := 10
	if len(serverConfig.List("app")) > 5 {
		amount = len(serverConfig.List("app")) * 2
	}
	appManager = cmdctrl.New(amount)

	if err = InitApps(); err != nil {
		logger.SysLog("error", "initializing app manager", err.Error())
		panic(err)
	}

	// set up http server
	server := NewServer()
	sysCfg := serverConfig.GetSysConfig()
	if sysCfg.Host == "" || sysCfg.Port == 0 {
		lAddr = serverHost.String() + ":" + strconv.Itoa(*serverPort)
	} else {
		lAddr = sysCfg.Host + ":" + strconv.Itoa(sysCfg.Port)
	}
	// listenAddr = serverHost.String() + ":" + strconv.Itoa(*serverPort)

	l, err := net.Listen("tcp", lAddr)
	defer func() {
		if err = l.Close(); err != nil {
			panic(err)
		}
	}()
	if err != nil {
		logger.SysLog("error", "setting http server", err.Error())
		panic(err)
	}

	logger.SysLog("info", "setting http server", fmt.Sprintf("server addr %s", lAddr))
	// add interrupt signal notification
	// start websocket manager
	// start http server
	go func() {
		s := <-close
		logger.SysLog("info", "stopping autotestagent", fmt.Sprintf("signal received: %s", s))
		appManager.StopAll()
		os.Exit(0)
	}()
	go wsManager.Run()
	if err := server.Serve(l); err != nil {
		logger.SysLog("error", "starting http server", err.Error())
		panic(err)
	}
}
