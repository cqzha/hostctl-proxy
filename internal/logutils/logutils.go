package logutils

import (
	"io"
	"net/http"
	"os"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type ServerLogger struct {
	log *logrus.Logger
}

func (sl *ServerLogger) Init(logFile *os.File) {
	sl.log = logrus.New()
	sl.log.SetOutput(io.MultiWriter(logFile, os.Stdout))
	sl.log.SetLevel(logrus.TraceLevel)
	sl.log.SetFormatter(&nested.Formatter{
		HideKeys: false,
	})
}

func (sl *ServerLogger) CommonLog(lvl, msg string, fields logrus.Fields) {
	e := sl.log.WithFields(fields)
	level, err := logrus.ParseLevel(lvl)
	if err != nil {
		panic(err)
	}
	e.Log(level, msg)
}

func (sl *ServerLogger) SysLog(lvl string, topic string, msg string) {
	f := logrus.Fields{
		"field": "system",
		"topic": topic,
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) ConfigLog(lvl string, topic string, msg string) {
	f := logrus.Fields{
		"field": "configuration",
		"topic": topic,
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) HttpRequestLog(lvl string, r *http.Request, msg string) {
	f := logrus.Fields{
		"field":          "http request",
		"request_url":    r.URL.String(),
		"request_method": r.Method,
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) HttpResponseLog(lvl string, msg string) {
	f := logrus.Fields{
		"field": "http response",
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) WebSocketLog(lvl string, ws *websocket.Conn, msg string) {
	var f logrus.Fields
	if ws == nil {
		f = logrus.Fields{
			"field": "http websocket",
		}
	} else {
		f = logrus.Fields{
			"field":          "http websocket",
			"remote_address": ws.RemoteAddr().String(),
			"local_address":  ws.LocalAddr().String(),
		}
	}

	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) SocketLog(lvl string, url string, msg string) {
	var f logrus.Fields
	f = logrus.Fields{
		"field":    "socket request",
		"conn_url": url,
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) AppLog(lvl string, topic string, app string, msg string) {
	f := logrus.Fields{
		"field": "app",
		"name":  app,
		"topic": topic,
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) ProxyLog(lvl string, topic string, pxy string, msg string) {
	f := logrus.Fields{
		"field": "proxy",
		"name":  pxy,
		"topic": topic,
	}
	sl.CommonLog(lvl, msg, f)
}

func (sl *ServerLogger) GetEntry(field logrus.Fields) *logrus.Entry {
	return sl.log.WithFields(field)
}
