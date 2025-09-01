package main

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type NotifyEvent struct {
	client  *WSClient
	message []byte
}

// 一个WSClient对应一次websocket connection
type WSClient struct {
	id      int64           // id暂时没用
	conn    *websocket.Conn // 连接指针
	manager *WSManager      // manager
	wch     chan []byte     // 写通道
}

func NewWSClient(wconn *websocket.Conn, manager *WSManager) *WSClient {
	now := time.Now()
	return &WSClient{
		id:      now.UnixNano() + int64(rand.Int()),
		conn:    wconn,
		manager: manager,
		wch:     make(chan []byte),
	}
}

// 读取客户端的信息
func (c *WSClient) ReadMsg() {
	defer func() {
		logger.WebSocketLog("info", c.conn, "Websocket disconnect")
		c.manager.RmWSClient(c)
	}()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.manager.notifyChan <- NotifyEvent{client: c, message: []byte("Bye!Bye!")}
			logger.WebSocketLog("error", c.conn, fmt.Sprintf("Failed to read from connection, %v", err))
			break
		}
		c.manager.notifyChan <- NotifyEvent{client: c, message: data}
	}
}

// 给客户端发信息
func (c *WSClient) WriteMsg() {
	defer func() {
		logger.WebSocketLog("info", c.conn, "Websocket disconnect")
		c.manager.RmWSClient(c)
	}()
	// 如果绑定了socket client则需要使用RealtimeReadPump
	if sc, ok := c.manager.sclients[c]; ok {
		go sc.RealtimeReadPump(c)
	}

	for {
		select {
		case data, ok := <-c.wch:
			if !ok {
				logger.WebSocketLog("error", c.conn, "Message channel closed, exiting handler")
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				logger.WebSocketLog("error", c.conn, fmt.Sprintf("Failed to send to connection, %v", err))
				return
			}
		}
	}
}

// socket client
// 用于连接socket server
type SocketClient struct {
	// wsclient *WSClient
	conn  *net.Conn
	url   string
	ctype string // 暂时用不着
}

func NewSocket(tp, socketUrl string) (*SocketClient, error) {
	conn, err := net.Dial("tcp", socketUrl)
	if err != nil {
		return nil, err
	}
	return &SocketClient{
		conn:  &conn,
		url:   socketUrl,
		ctype: tp,
	}, nil
}

// 向server发送消息
func (sc *SocketClient) WritePump(input []byte) error {
	if _, err := (*sc.conn).Write(input); err != nil {
		logger.SocketLog("error", sc.url, fmt.Sprintf("Failed to send message to server: %v", err))
		return err
	}
	return nil
}

// 接收server的消息
func (sc *SocketClient) ReadPump() ([]byte, error) {
	output := make([]byte, 1024)
	if _, err := (*sc.conn).Read(output); err != nil {
		return nil, err
	}
	return output, nil
}

// 实时接收server的消息
func (sc *SocketClient) RealtimeReadPump(c *WSClient) {
	defer func() {
		logger.SocketLog("info", sc.url, "Socket disconnected")
		(*sc.conn).Close()
	}()

	for {
		output, err := sc.ReadPump()
		if err != nil {
			logger.SocketLog("error", sc.url, fmt.Sprintf("Failed to receive message from server: %v", err))
			c.wch <- nil
			break
		}
		c.wch <- output
	}
}

// 用于管理websocket连接
type WSManager struct {
	wsclients    map[*WSClient]bool          // 已注册的websocket connection
	sclients     map[*WSClient]*SocketClient // websocket对应的socket client，如果有的话
	sync.RWMutex                             // 互斥锁
	notifyChan   chan NotifyEvent            // 通知channel
}

func NewWebsocketManager() *WSManager {
	return &WSManager{
		wsclients:  make(map[*WSClient]bool),
		sclients:   make(map[*WSClient]*SocketClient),
		notifyChan: make(chan NotifyEvent),
	}
}

// 移除已注册的websocket connection
// 一般在客户端断开连接后使用
// 使用时需要加互斥锁，防止goroutine之间竞争
func (m *WSManager) RmWSClient(client *WSClient) {
	m.RLock()
	defer m.RUnlock()
	if _, ok := m.wsclients[client]; ok {
		// 如果该websocket connection有socket client时，需要先关闭socket client
		if sc, ok := m.sclients[client]; ok {
			(*sc.conn).Close()
			delete(m.sclients, client)
		}
		client.conn.Close()
		delete(m.wsclients, client)
	}
}

// 注册新的websocket connection
func (m *WSManager) AddWSClient(client *WSClient) {
	m.RLock()
	defer m.RUnlock()
	m.wsclients[client] = true
}

// 移除websocket connection对应的socket client
// 至于为什么不把socket conn直接放WSClient结构体中
// 为了降低结构体之间的耦合程度
func (m *WSManager) RmSockClient(c *WSClient) {
	m.RLock()
	defer m.RUnlock()
	if sc, ok := m.sclients[c]; ok {
		(*sc.conn).Close()
		delete(m.sclients, c)
	}
}

// 为websocket connection注册新的socket client
func (m *WSManager) AddSockClient(c *WSClient, tp, socketUrl string) error {
	m.RLock()
	defer m.RUnlock()
	sc, err := NewSocket(tp, socketUrl)
	if err != nil {
		return err
	}
	m.sclients[c] = sc
	return nil
}

// websocket connection和socket client建立绑定关系后
// websocket 的消息通过此方法发给socket client
func (m *WSManager) NotifySock(evt NotifyEvent) error {
	client := evt.client
	msg := evt.message
	if sc, ok := m.sclients[client]; ok {
		if err := sc.WritePump(msg); err != nil {
			return err
		}
		return nil
	} else {
		return fmt.Errorf("%#v has not bind any socket client", client)
	}
}

// manager的通信线程
func (m *WSManager) Run() {
	for {
		select {
		case e := <-m.notifyChan:
			m.NotifySock(e)
		}
	}
}
