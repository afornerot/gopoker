package main

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var FibonacciValues = []string{"0", "1", "2", "3", "5", "8", "13", "21", "34", "55", "89", "?"}

type Client struct {
	Conn    *websocket.Conn
	Room    *Room
	Name    string
	IsAdmin bool
	Send    chan []byte
}

type Hub struct {
	mu    sync.RWMutex
	Rooms map[string]*Room
}

var hub = &Hub{Rooms: make(map[string]*Room)}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type clientMessage struct {
	Type  string  `json:"type"`
	Value *string `json:"value,omitempty"`
	Name  string  `json:"name,omitempty"`
}

type RoomSummary struct {
	ID               string
	Name             string
	ParticipantCount int
	AdminName        string
	CreatedAt        time.Time
}

func (h *Hub) CreateRoom(name string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := generateRoomID()
	for h.Rooms[id] != nil {
		id = generateRoomID()
	}
	room := NewRoom(id, name)
	h.Rooms[id] = room
	return room
}

func (h *Hub) GetRoom(id string) *Room {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Rooms[id]
}

func (h *Hub) ListRooms() []RoomSummary {
	h.mu.RLock()
	rooms := make([]RoomSummary, 0, len(h.Rooms))
	for _, r := range h.Rooms {
		r.mu.RLock()
		count := len(r.Participants)
		admin := ""
		for _, p := range r.Participants {
			if p.IsAdmin {
				admin = p.Name
				break
			}
		}
		name := r.Name
		created := r.CreatedAt
		id := r.ID
		r.mu.RUnlock()
		if name == "" {
			name = "Salon " + id
		}
		rooms = append(rooms, RoomSummary{
			ID:               id,
			Name:             name,
			ParticipantCount: count,
			AdminName:        admin,
			CreatedAt:        created,
		})
	}
	h.mu.RUnlock()

	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].CreatedAt.After(rooms[j].CreatedAt)
	})
	return rooms
}

func (h *Hub) buildState(room *Room) map[string]interface{} {
	room.mu.RLock()
	defer room.mu.RUnlock()

	participants := make([]map[string]interface{}, 0)
	for _, p := range room.Participants {
		pp := map[string]interface{}{
			"name":      p.Name,
			"hasVoted":  p.Vote != nil,
			"connected": p.Connected,
			"isAdmin":   p.IsAdmin,
		}
		if room.Revealed {
			pp["vote"] = p.Vote
		} else {
			pp["vote"] = nil
		}
		participants = append(participants, pp)
	}

	return map[string]interface{}{
		"type":         "state",
		"participants": participants,
		"revealed":     room.Revealed,
	}
}

func (h *Hub) broadcastToRoom(room *Room, msg map[string]interface{}, exclude ...*Client) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("marshal error:", err)
		return
	}
	room.mu.RLock()
	for client := range room.Clients {
		if len(exclude) > 0 && client == exclude[0] {
			continue
		}
		select {
		case client.Send <- data:
		default:
			close(client.Send)
			delete(room.Clients, client)
		}
	}
	room.mu.RUnlock()
}

func (h *Hub) broadcastState(room *Room) {
	state := h.buildState(room)
	h.broadcastToRoom(room, state)
}

func (h *Hub) HandleWebSocket(conn *websocket.Conn, room *Room, name string, isAdmin bool) {
	if !room.NameAvailable(name) {
		msg, _ := json.Marshal(map[string]string{"type": "error", "message": "Nom déjà utilisé"})
		conn.WriteMessage(websocket.TextMessage, msg)
		conn.Close()
		return
	}

	client := &Client{
		Conn: conn,
		Room: room,
		Name: name,
		Send: make(chan []byte, 256),
	}

	room.ConnectParticipant(client, name, isAdmin)

	go client.writePump()

	state := h.buildState(room)
	data, _ := json.Marshal(state)
	client.Send <- data

	hub.broadcastToRoom(room, map[string]interface{}{
		"type": "user_joined",
		"name": name,
	}, client)
	hub.broadcastToRoom(room, state, client)

	client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.Room.HandleDisconnect(c)
		c.Conn.Close()

		hub.broadcastToRoom(c.Room, map[string]interface{}{
			"type": "user_left",
			"name": c.Name,
		})
		hub.broadcastState(c.Room)
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg clientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "vote":
			isRevealed := func() bool {
				c.Room.mu.RLock()
				defer c.Room.mu.RUnlock()
				return c.Room.Revealed
			}()

			if isRevealed {
				c.Room.SetVote(c.Name, msg.Value)
				hub.broadcastState(c.Room)
			} else {
				ok := c.Room.SetVote(c.Name, msg.Value)
				if ok {
					hub.broadcastState(c.Room)
				}
			}

		case "reveal":
			if !c.IsAdmin {
				continue
			}
			c.Room.Lock()
			hub.broadcastState(c.Room)

		case "reset":
			if !c.IsAdmin {
				continue
			}
			c.Room.Unlock()
			hub.broadcastState(c.Room)

		case "remove":
			if !c.IsAdmin || msg.Name == "" {
				continue
			}
			if c.Room.RemoveDisconnected(msg.Name) {
				hub.broadcastState(c.Room)
			}

		case "promote":
			if !c.IsAdmin || msg.Name == "" {
				continue
			}
			if c.Room.PromoteAdmin(msg.Name) {
				hub.broadcastState(c.Room)
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}