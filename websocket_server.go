package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mark3labs/mcp-go/server"
)

type WebSocketServerTransportOption func(*websocketServerTransport)

type websocketServerTransport struct {
	server   *server.MCPServer
	conn     *websocket.Conn
	endpoint string

	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup
}

func WithWebSocketServerOptionMcpServer(server *server.MCPServer) WebSocketServerTransportOption {
	return func(t *websocketServerTransport) {
		t.server = server
	}
}

func NewWebSocketServerTransport(endpoint string, opts ...WebSocketServerTransportOption) (*websocketServerTransport, error) {
	t := &websocketServerTransport{
		endpoint: endpoint,
	}

	for _, opt := range opts {
		opt(t)
	}

	// 创建WebSocket连接
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket endpoint: %w", err)
	}
	t.conn = conn

	return t, nil
}

func (t *websocketServerTransport) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel

	// 启动心跳检测
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := t.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					fmt.Println("ping error:", err)
					t.cancel()
					return
				}
			case <-t.ctx.Done():
				return
			}
		}
	}()

	// 主消息处理循环
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		for {
			select {
			case <-t.ctx.Done():
				return
			default:
				_, message, err := t.conn.ReadMessage()
				if err != nil {
					fmt.Println("read message error:", err)
					t.cancel()
					return
				}
				fmt.Println("recv message:", string(message))

				response := t.server.HandleMessage(t.ctx, message)
				if response != nil {
					responseBytes, err := json.Marshal(response)
					if err != nil {
						fmt.Println("marshal message error:", err)
						t.cancel()
						return
					}
					fmt.Println("send message:", string(responseBytes))
					err = t.conn.WriteMessage(websocket.TextMessage, responseBytes)
					if err != nil {
						fmt.Println("write message error:", err)
						t.cancel()
						return
					}
				}
			}
		}
	}()

	// 等待所有goroutine完成
	t.wg.Wait()
	return nil
}
