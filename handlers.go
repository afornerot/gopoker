package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PostFormValue("name"))
	if len(name) > 40 {
		name = name[:40]
	}
	room := hub.CreateRoom(name)
	http.Redirect(w, r, fmt.Sprintf("/room/%s?admin=%s", room.ID, room.AdminToken), http.StatusSeeOther)
}

func handleRoomPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	room := hub.GetRoom(id)
	if room == nil {
		http.Redirect(w, r, "/?error=room_not_found", http.StatusSeeOther)
		return
	}

	adminToken := r.URL.Query().Get("admin")
	isValidAdmin := room.IsAdmin(adminToken)

	data := pageData{
		ID:         id,
		AdminToken: adminToken,
		IsAdmin:    isValidAdmin,
		RoomName:   room.Name,
		Fibonacci:  FibonacciValues,
	}

	if err := templates.ExecuteTemplate(w, "room.html", data); err != nil {
		log.Println("template error:", err)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	room := hub.GetRoom(id)
	if room == nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	adminToken := r.URL.Query().Get("admin")
	isAdmin := room.IsAdmin(adminToken)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}

	hub.HandleWebSocket(conn, room, name, isAdmin)
}