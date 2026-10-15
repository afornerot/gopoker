package main

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"
)

type Participant struct {
	Name      string  `json:"name"`
	Vote      *string `json:"vote"`
	Connected bool    `json:"connected"`
	IsAdmin   bool    `json:"isAdmin"`
	Seq       uint64  `json:"-"`
}

type Room struct {
	mu           sync.RWMutex
	ID           string
	Name         string
	CreatedAt    time.Time
	AdminToken   string
	Participants map[string]*Participant
	Clients      map[*Client]bool
	Revealed     bool
	seq          uint64
}

func NewRoom(id, name string) *Room {
	return &Room{
		ID:           id,
		Name:         name,
		CreatedAt:    time.Now(),
		AdminToken:   generateToken(32),
		Participants: make(map[string]*Participant),
		Clients:      make(map[*Client]bool),
	}
}

func (r *Room) NameAvailable(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.Participants[name]
	if !ok {
		return true
	}
	return !p.Connected
}

func (r *Room) SetVote(name string, value *string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.Participants[name]; ok {
		p.Vote = value
		return true
	}
	return false
}

func (r *Room) IsAdmin(token string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return token != "" && token == r.AdminToken
}

func (r *Room) Lock() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Revealed = true
}

func (r *Room) Unlock() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Revealed = false
	for _, p := range r.Participants {
		p.Vote = nil
	}
}

// RemoveDisconnected removes a disconnected participant. Returns false if the
// participant does not exist or is still connected.
func (r *Room) RemoveDisconnected(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.Participants[name]
	if !ok || p.Connected {
		return false
	}
	delete(r.Participants, name)
	return true
}

// PromoteAdmin transfers the admin role to a connected participant.
// The current admin keeps their connection but loses admin rights.
func (r *Room) PromoteAdmin(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	target, ok := r.Participants[name]
	if !ok || !target.Connected {
		return false
	}
	for _, p := range r.Participants {
		p.IsAdmin = p == target
	}
	for c := range r.Clients {
		c.IsAdmin = c.Name == name
	}
	return true
}

// MarkDisconnectedAndTransfer marks the leaving client offline and, if it was
// the admin, promotes the first connected participant (earliest join).
// Must be called with the room lock held.
func (r *Room) markDisconnected(c *Client, promoteOnLeave bool) {
	delete(r.Clients, c)
	if p, ok := r.Participants[c.Name]; ok {
		p.Connected = false
		if promoteOnLeave {
			p.IsAdmin = false
		}
	}
	if !promoteOnLeave {
		return
	}
	var next string
	var minSeq uint64 = ^uint64(0)
	for _, pp := range r.Participants {
		if pp.Connected && pp.Seq < minSeq {
			minSeq = pp.Seq
			next = pp.Name
		}
	}
	if next != "" {
		r.Participants[next].IsAdmin = true
		for cc := range r.Clients {
			cc.IsAdmin = cc.Name == next
		}
	}
}

// HandleDisconnect is called by the read pump when a client leaves.
func (r *Room) HandleDisconnect(c *Client) {
	r.mu.Lock()
	r.markDisconnected(c, c.IsAdmin)
	r.mu.Unlock()
}

// ConnectParticipant registers a new or reconnecting client. When the caller
// holds a valid admin token, the admin role is transferred to them, ensuring a
// single admin at all times.
func (r *Room) ConnectParticipant(c *Client, name string, isAdmin bool) {
	r.mu.Lock()
	r.seq++
	if p, ok := r.Participants[name]; ok {
		p.Connected = true
	} else {
		p = &Participant{Name: name, Connected: true, Seq: r.seq}
		r.Participants[name] = p
	}
	if isAdmin {
		for _, pp := range r.Participants {
			pp.IsAdmin = pp.Name == name
		}
		for cc := range r.Clients {
			cc.IsAdmin = false
		}
		r.Participants[name].IsAdmin = true
		c.IsAdmin = true
	} else if r.Participants[name].IsAdmin {
		c.IsAdmin = true
	}
	r.Clients[c] = true
	r.mu.Unlock()
}

func generateToken(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func generateRoomID() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}