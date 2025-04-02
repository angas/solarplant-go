package www

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client struct {
	logger *slog.Logger
	hub    *Hub
	conn   *ws.Conn
	send   chan []byte
	name   string
}

func NewClient(hub *Hub, w http.ResponseWriter, r *http.Request, name string) (*Client, error) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}

	return &Client{
		logger: hub.logger.With(slog.String("client", name)),
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		name:   name,
	}, nil
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		c.hub.Unregister <- c
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Warn("web socket set write deadline failed", slog.Any("error", err))
				return
			}

			if !ok {
				if err := c.conn.WriteMessage(ws.CloseMessage, []byte{}); err != nil {
					c.logger.Debug("web socket close message failed", slog.Any("error", err))
				}
				return
			}

			w, err := c.conn.NextWriter(ws.TextMessage)
			if err != nil {
				c.logger.Warn("web socket next writer failed", slog.Any("error", err))
				return
			}

			if _, err = w.Write(message); err != nil {
				c.logger.Warn("web socket write failed", slog.Any("error", err))
				return
			}

			if err = w.Close(); err != nil {
				c.logger.Debug("web socket close failed", slog.Any("error", err))
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Warn("web socket set write deadline failed", slog.Any("error", err))
				return
			}
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				c.logger.Warn("web socket ping message failed", slog.Any("error", err))
				return
			}
		}
	}
}

// Hub maintains the set of active clients and broadcasts messages to clients
type Hub struct {
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	clients    map[*Client]bool
	mutex      sync.Mutex
	logger     *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		Broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		logger:     logger,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.logger.Debug("registering client", "clientName", client.name)

			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()

		case client := <-h.Unregister:
			h.logger.Debug("unregistering client", "clientName", client.name)

			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()

		case message := <-h.Broadcast:
			// Create a temporary slice of clients while holding the lock
			h.mutex.Lock()
			activeClients := make([]*Client, 0, len(h.clients))
			for client := range h.clients {
				activeClients = append(activeClients, client)
			}
			h.mutex.Unlock()

			for _, client := range activeClients {
				select {
				case client.send <- message:
				default: // Client's channel is full, drop the message
					h.logger.Warn("client send buffer full, dropping message", "clientName", client.name)
				}
			}
		}
	}
}
