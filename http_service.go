package main

import (
	"hostctl_proxy/internal/config"
	"hostctl_proxy/internal/command"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	//"io/ioutil"
	"net"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

type HostSwitch map[string]http.Handler
type Server struct {
	httpServer *http.Server
}

type BodyExec struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

type BodyWithArgs struct {
	Args []string `json:"args"`
}

type BodyProxy struct {
	Host    string `json:"host"`
	Port    int    `json:"Port"`
	Content string `json:"content"`
}

// type PutBody struct {
// 	Control string
// }

func RenderJSON(w http.ResponseWriter, flg bool, result interface{}) {
	res := make(map[string]interface{})
	if flg {
		res["code"] = 0
	} else {
		res["code"] = -1
	}

	jsRes, err := AppoutJsonSerialize(result)
	if err != nil {
		data := make(map[string]interface{})
		if flg {
			data["output"] = result
		} else {
			data["msg"] = result
		}
		res["data"] = data
	} else {
		res["data"] = jsRes
	}
	js, err := json.Marshal(res)
	if err != nil {
		logger.HttpResponseLog("error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(js)))
	w.WriteHeader(http.StatusOK)
	if c, err := w.Write(js); err != nil {
		logger.HttpResponseLog("error", err.Error())
	} else {
		logger.HttpResponseLog("info", fmt.Sprintf("sent ok, size: %v, body: %s", c, js))
	}
}

func RequestPreprocess(handler func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		logger.HttpRequestLog("info", r, "request received")
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			// io.ReadAll会导致request body不能读第二次
			// 用io.TeeReader解决上述问题
			buf := bytes.Buffer{}
			tee := io.TeeReader(r.Body, &buf)
			r.Body = io.NopCloser(&buf)
			if _, err := io.ReadAll(tee); err != nil {
				logger.HttpRequestLog("error", r, err.Error())
				http.Error(w, err.Error(), 400)
				return
			}
		}

		handler(w, r, p)
	}
}

func SocketTunnel(url string, data []byte, ch chan string) {
	conn, err := net.Dial("tcp", url)
	if err != nil {
		logger.SocketLog("error", url, err.Error())
		ch <- err.Error()
		return
	}

	defer func() {
		if err = conn.Close(); err != nil {
			logger.SocketLog("error", url, err.Error())
			ch <- err.Error()
		}
	}()
	err = conn.SetDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		logger.SocketLog("error", url, err.Error())
		ch <- err.Error()
		return
	}
	conn.Write(data)
	reader := bufio.NewReader(conn)
	var out string
	// buf := make([]byte, 1024)
	for {
		//n, err := conn.Read(buf)
		line, _, err := reader.ReadLine()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
		out = out + fmt.Sprintf("\n%s", line)
		//out = out + string(buf[:n])
	}
	out = strings.Trim(out, "\n")
	fmt.Printf("received all")
	ch <- out
}

func (server *Server) initHttpServer() {
	router := httprouter.New()
	router.GET("/", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		RenderJSON(w, true, "OK! service is active")
	}))

	router.Handle(http.MethodGet, "/list/:components", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		components := p.ByName("components")
		var data interface{}
		if components == "command" {
			data = serverConfig.List("command")
		} else if components == "app" {
			data = serverConfig.List("app")
			// status := r.URL.Query().Get("status")
			// if status ==
		} else {
			data = serverConfig.List("all")
		}
		RenderJSON(w, true, data)
	}))

	router.Handle(http.MethodPut, "/configure", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := serverConfig.Dump(jsonPath); err != nil {
			RenderJSON(w, false, err.Error())
			return
		}

		RenderJSON(w, true, "OK! config file is updated")
	}))

	router.Handle(http.MethodPost, "/configure/:field/:name", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		field := p.ByName("field")
		name := p.ByName("name")

		// 先增加配置
		if err := serverConfig.Add(field, name, data); err != nil {
			logger.ConfigLog("error", fmt.Sprintf("adding %s to config", name), err.Error())
			RenderJSON(w, false, err.Error())
			return
		}

		// 如果配置的是app，先加到配置manager，再增加到app manager
		if field == "app" {
			appCfg := serverConfig.GetConfig("app", name).(*config.AppCfg)
			cmdInfo, err := ConvertAppConfig(name, appCfg)
			if err != nil {
				logger.AppLog("error", "configuring", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}

			if err = appManager.Add(name, cmdInfo); err != nil {
				logger.AppLog("error", "adding", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}
			logger.AppLog("info", "adding", name, fmt.Sprintf("%s is added", name))
		}
		RenderJSON(w, true, fmt.Sprintf("OK! %s: %s is added", field, name))
	}))

	router.Handle(http.MethodGet, "/configure/:field/:name", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		field := p.ByName("field")
		name := p.ByName("name")
		cfg := serverConfig.GetConfig(field, name)
		if cfg == nil {
			RenderJSON(w, false, fmt.Errorf("%s not found: %s", field, name).Error())
			return
		}
		RenderJSON(w, true, cfg)
	}))

	router.Handle(http.MethodDelete, "/configure/:field/:name", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		field := p.ByName("field")
		name := p.ByName("name")
		if field == "app" && appManager.Exists(name) {
			if err := appManager.Remove(name); err != nil {
				logger.AppLog("error", "removing", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}
		}

		if err := serverConfig.Delete(field, name); err != nil {
			logger.ConfigLog("error", fmt.Sprintf("removing %s from config", name), err.Error())
			RenderJSON(w, false, err.Error())
			return
		}
		RenderJSON(w, true, fmt.Sprintf("OK! %s: %s is deleted", field, name))
	}))

	router.Handle(http.MethodPut, "/configure/:field/:name", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		field := p.ByName("field")
		name := p.ByName("name")

		if field == "app" && appManager.Exists(name) {
			if err := appManager.Remove(name); err != nil {
				logger.AppLog("error", "removing", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}
		}

		if err := serverConfig.Modify(field, name, data); err != nil {
			logger.ConfigLog("error", fmt.Sprintf("editing %s config", name), err.Error())
			RenderJSON(w, false, err.Error())
			return
		}

		if field == "app" {
			appCfg := serverConfig.GetConfig("app", name).(*config.AppCfg)
			cmdInfo, err := ConvertAppConfig(name, appCfg)
			if err != nil {
				logger.AppLog("error", "configuring", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}

			if err = appManager.Add(name, cmdInfo); err != nil {
				logger.AppLog("error", "adding", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}
		}
		RenderJSON(w, true, fmt.Sprintf("OK! %s: %s is modified", field, name))
	}))

	router.Handle(http.MethodPost, "/exec", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		var rdata BodyExec
		if err := json.Unmarshal(data, &rdata); err != nil {
			logger.HttpRequestLog("error", r, err.Error())
			http.Error(w, err.Error(), 400)
			return
		}

		cmd := command.Command{
			Args:    append([]string{rdata.Cmd}, rdata.Args...),
			Shell:   true,
			Timeout: 10 * time.Minute,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			RenderJSON(w, false, err.Error())
		} else {
			RenderJSON(w, true, strings.TrimSpace(string(output)))
		}
	}))

	router.Handle(http.MethodPost, "/command/:name", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		cmdName := p.ByName("name")
		cmdCfg := serverConfig.GetConfig("command", cmdName).(*config.CmdCfg)
		var rdata BodyWithArgs
		if err := json.Unmarshal(data, &rdata); err != nil {
			logger.HttpRequestLog("error", r, err.Error())
			http.Error(w, err.Error(), 400)
			return
		}

		var args []string
		if len(rdata.Args) > 0 {
			args = rdata.Args
		} else {
			args = cmdCfg.DefaultArgs
		}

		cmd := command.Command{
			Args:    append([]string{cmdCfg.Cmd}, args...),
			Shell:   true,
			Timeout: 10 * time.Minute,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			RenderJSON(w, false, err.Error())
		} else {
			RenderJSON(w, true, strings.TrimSpace(string(output)))
		}
	}))

	router.Handle(http.MethodGet, "/app/status", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := r.URL.Query().Get("name")
		if !appManager.Exists(name) {
			RenderJSON(w, false, fmt.Sprintf("app not found: %s", name))
			return
		}
		var status string
		if appManager.Running(name) {
			status = "running"
		} else {
			status = "stopped"
		}

		RenderJSON(w, true, status)
	}))

	router.Handle(http.MethodPost, "/app/control", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		name := r.URL.Query().Get("name")
		var rdata BodyWithArgs
		if err := json.Unmarshal(data, &rdata); err != nil {
			logger.HttpRequestLog("error", r, err.Error())
			http.Error(w, err.Error(), 400)
			return
		}
		if err := appManager.Start(name, rdata.Args...); err != nil {
			errMsg := fmt.Sprintf("Fail! app %s %s", name, err.Error())
			logger.AppLog("error", "starting", name, errMsg)
			RenderJSON(w, false, errMsg)
			return
		}
		RenderJSON(w, true, fmt.Sprintf("OK! app %s is started", name))
	}))

	router.Handle(http.MethodDelete, "/app/control", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := r.URL.Query().Get("name")
		if sockpManager.Exists(name) {
			if err := sockpManager.Deregister(name); err != nil {
				logger.AppLog("error", "socketport deregistering", name, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}
		}

		if err := appManager.Stop(name, true); err != nil {
			logger.AppLog("error", "stopping", name, err.Error())
			RenderJSON(w, false, err.Error())
			return
		}

		RenderJSON(w, true, fmt.Sprintf("OK! app %s is stopped", name))
	}))

	router.Handle(http.MethodGet, "/app/link/:appname", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		appName := p.ByName("appname")
		appCfg := serverConfig.GetConfig("app", appName).(*config.AppCfg)
		if appCfg.Websocket {
			wconn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				logger.HttpRequestLog("error", r, "failed to upgrade request")
				http.Error(w, "Failed to upgrade request", 500)
				return
			}
			socketUrl, err := GetSocketUrl(appName)
			if err != nil {
				logger.AppLog("error", "getting socketurl", appName, err.Error())
				http.Error(w, fmt.Sprintf("Failed to get %s's socketurl", appName), 500)
				return
			}
			client := NewWSClient(wconn, wsManager)
			wsManager.AddWSClient(client)
			if err = wsManager.AddSockClient(client, "recv", socketUrl); err != nil {
				logger.AppLog("error", "adding socketclient", appName, err.Error())
				http.Error(w, fmt.Sprintf("Failed to add %s's socketclient", appName), 500)
				return
			}
			go client.ReadMsg()
			go client.WriteMsg()
		} else {
			err := fmt.Errorf("app %s method not allowed", appName)
			logger.HttpRequestLog("error", r, err.Error())
			http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		}
	})

	router.Handle(http.MethodPut, "/app/link/:appname", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		appName := p.ByName("appname")
		appCfg := serverConfig.GetConfig("app", appName).(*config.AppCfg)
		logger.AppLog("info", "interacting", appName, fmt.Sprintf("request body: %s", data))
		if appCfg.Socket {
			socketUrl, err := GetSocketUrl(appName)
			if err != nil {
				logger.AppLog("error", "getting socketurl", appName, err.Error())
				RenderJSON(w, false, err.Error())
				return
			}
			channel := make(chan string)
			/* 			go func(url string, data []byte, ch chan string) {
				conn, err := net.Dial("tcp", url)
				defer func() {
					if err = conn.Close(); err != nil {
						logger.SocketLog("error", &conn, err.Error())
						ch <- err.Error()
					}
				}()
				if err != nil {
					logger.SocketLog("error", &conn, err.Error())
					ch <- err.Error()
				}

				conn.Write(data)
				reader := bufio.NewReader(conn)
				var out string
				for {
					line, _, err := reader.ReadLine()
					if err != nil {
						break
					}
					out = out + fmt.Sprintf("\n%s", line)
				}
				out = strings.Trim(out, "\n")
				ch <- out
			}(socketUrl, data, channel) */
			go SocketTunnel(socketUrl, data, channel)
			output := <-channel
			header := output[0:4]
			if header == "0000" {
				RenderJSON(w, false, output[4:])
			} else {
				RenderJSON(w, true, output[4:])
			}
		} else {
			http.Error(w, fmt.Sprintf("%s does not support link", appName), http.StatusMethodNotAllowed)
		}
	}))

	// 临时用，后续用反向代理做转发
	router.Handle(http.MethodPut, "/proxy/:name", RequestPreprocess(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		data, _ := io.ReadAll(r.Body)
		pxyName := p.ByName("name")
		var pxyCfg *config.ProxyCfg
		_pxyCfg := serverConfig.GetConfig("proxy", pxyName)

		var rdata BodyProxy
		if err := json.Unmarshal(data, &rdata); err != nil {
			logger.HttpRequestLog("error", r, err.Error())
			http.Error(w, err.Error(), 400)
			return
		}

		var url string
		if _pxyCfg == nil {
			// 如果没有对应名称，则直接用request body中的host和port作为url发请求
			if rdata.Host == "" && rdata.Port == 0 {
				logger.ProxyLog("error", "getting proxy info", pxyName, "host and port are empty")
				RenderJSON(w, false, "host and port are empty")
				return
			}
			url = fmt.Sprintf("%v:%v", rdata.Host, rdata.Port)
		} else {
			pxyCfg = _pxyCfg.(*config.ProxyCfg)
			url = fmt.Sprintf("%v:%v", pxyCfg.Host, pxyCfg.Port)
		}
		channel := make(chan string)
		go SocketTunnel(url, []byte(rdata.Content), channel)
		output := <-channel
		RenderJSON(w, true, output)
	}))
	// 服务热重启
	// router.Handle(http.MethodPut, "/restart", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// })

	server.httpServer = &http.Server{Handler: router}
}

func (server *Server) Serve(l net.Listener) error {
	return server.httpServer.Serve(l)
}
