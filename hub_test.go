package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestE2EPokerPlanning(t *testing.T) {
	hub = &Hub{Rooms: make(map[string]*Room)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", handleCreateRoom)
	mux.HandleFunc("GET /room/{id}", handleRoomPage)
	mux.HandleFunc("GET /ws/{id}", handleWebSocket)

	server := httptest.NewServer(mux)
	defer server.Close()

	httpURL := server.URL
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http")

	// 1. Create room (no redirect)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	createResp, err := client.Post(httpURL+"/create", "", nil)
	if err != nil {
		t.Fatal("create room:", err)
	}
	location := createResp.Header.Get("Location")
	if location == "" {
		t.Fatal("no redirect after create")
	}
	roomID := extractParam(location, "/room/", "?")
	adminToken := extractParam(location, "admin=", "")

	t.Logf("Room created: %s", roomID)

	// 2. Admin connects
	adminURL := url.URL{Scheme: "ws", Host: strings.TrimPrefix(wsURL, "ws://"),
		Path: "/ws/" + roomID, RawQuery: "name=Alice&admin=" + adminToken}
	admin, _, err := websocket.DefaultDialer.Dial(adminURL.String(), nil)
	if err != nil {
		t.Fatal("admin dial:", err)
	}
	defer admin.Close()

	state := readWS(t, admin, "admin initial state")
	assertState(t, state, 1, false)

	// 3. Bob joins (participant)
	bobURL := url.URL{Scheme: "ws", Host: strings.TrimPrefix(wsURL, "ws://"),
		Path: "/ws/" + roomID, RawQuery: "name=Bob"}
	bob, _, err := websocket.DefaultDialer.Dial(bobURL.String(), nil)
	if err != nil {
		t.Fatal("bob dial:", err)
	}
	defer bob.Close()

	state = readWS(t, bob, "bob initial state")
	assertState(t, state, 2, false)

	event := readWS(t, admin, "admin user_joined")
	if event["type"] != "user_joined" || event["name"] != "Bob" {
		t.Fatalf("expected user_joined Bob, got %v", event)
	}

	// Admin must see the updated participant list without any vote
	adminState := readWS(t, admin, "admin participants after join")
	assertState(t, adminState, 2, false)
	assertParticipant(t, adminState, "Bob", false, nil)

	// 4. Bob votes 5
	writeWS(t, bob, map[string]interface{}{"type": "vote", "value": "5"})
	state = readWS(t, bob, "bob state after vote")
	assertState(t, state, 2, false)
	assertParticipant(t, state, "Bob", true, nil)

	state = readWS(t, admin, "admin sees bob voted")
	assertState(t, state, 2, false)
	assertParticipant(t, state, "Bob", true, nil)

	// 5. Admin votes 8
	writeWS(t, admin, map[string]interface{}{"type": "vote", "value": "8"})
	state = readWS(t, admin, "admin state after vote")
	assertParticipant(t, state, "Alice", true, nil)

	state = readWS(t, bob, "bob sees alice voted")
	assertParticipant(t, state, "Alice", true, nil)

	// 6. Admin reveals
	writeWS(t, admin, map[string]interface{}{"type": "reveal"})
	state = readWS(t, admin, "admin sees revealed")
	assertState(t, state, 2, true)
	assertParticipant(t, state, "Alice", true, strPtr("8"))
	assertParticipant(t, state, "Bob", true, strPtr("5"))

	state = readWS(t, bob, "bob sees revealed")
	assertState(t, state, 2, true)
	assertParticipant(t, state, "Bob", true, strPtr("5"))

	// 7. Bob changes his vote after reveal
	writeWS(t, bob, map[string]interface{}{"type": "vote", "value": "8"})
	state = readWS(t, admin, "admin sees bob changed vote")
	assertParticipant(t, state, "Bob", true, strPtr("8"))

	state = readWS(t, bob, "bob sees own change")
	assertParticipant(t, state, "Bob", true, strPtr("8"))

	// 8. Admin resets
	writeWS(t, admin, map[string]interface{}{"type": "reset"})
	state = readWS(t, admin, "admin sees reset")
	assertState(t, state, 2, false)
	assertParticipant(t, state, "Alice", false, nil)
	assertParticipant(t, state, "Bob", false, nil)

	state = readWS(t, bob, "bob sees reset")
	assertState(t, state, 2, false)

	// 9. Duplicate name rejected
	dupURL := url.URL{Scheme: "ws", Host: strings.TrimPrefix(wsURL, "ws://"),
		Path: "/ws/" + roomID, RawQuery: "name=Bob"}
	dup, _, err := websocket.DefaultDialer.Dial(dupURL.String(), nil)
	if err != nil {
		t.Fatal("dup dial should succeed:", err)
	}
	dup.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := dup.ReadMessage()
	if err != nil {
		t.Fatal("dup should receive error:", err)
	}
	var errMsg map[string]interface{}
	json.Unmarshal(msg, &errMsg)
	if errMsg["type"] != "error" {
		t.Fatalf("expected error type, got %v", errMsg["type"])
	}
	dup.Close()
	t.Logf("Duplicate name correctly rejected")

	// 10. Non-admin cannot reveal
	writeWS(t, bob, map[string]interface{}{"type": "reveal"})
	// No response expected — Bob votes again to trigger a state update
	writeWS(t, bob, map[string]interface{}{"type": "vote", "value": "13"})
	state = readWS(t, bob, "bob votes after failed reveal")
	if state["revealed"] == true {
		t.Fatal("non-admin should not be able to reveal")
	}
	t.Logf("Non-admin reveal correctly ignored")
}

func readWS(t *testing.T, c *websocket.Conn, label string) map[string]interface{} {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read %s: %v", label, err)
	}
	var data map[string]interface{}
	json.Unmarshal(msg, &data)
	t.Logf("  %s: type=%s", label, data["type"])
	return data
}

func writeWS(t *testing.T, c *websocket.Conn, msg map[string]interface{}) {
	t.Helper()
	if err := c.WriteJSON(msg); err != nil {
		t.Fatalf("write %v: %v", msg, err)
	}
}

func assertState(t *testing.T, data map[string]interface{}, numParticipants int, revealed bool) {
	t.Helper()
	if data["type"] != "state" {
		t.Fatalf("expected state, got %s", data["type"])
	}
	parts := data["participants"].([]interface{})
	if len(parts) != numParticipants {
		t.Fatalf("expected %d participants, got %d", numParticipants, len(parts))
	}
	if data["revealed"] != revealed {
		t.Fatalf("expected revealed=%v, got %v", revealed, data["revealed"])
	}
}

func assertParticipant(t *testing.T, data map[string]interface{}, name string, hasVoted bool, vote *string) {
	t.Helper()
	parts := data["participants"].([]interface{})
	for _, p := range parts {
		pp := p.(map[string]interface{})
		if pp["name"] == name {
			if pp["hasVoted"] != hasVoted {
				t.Fatalf("participant %s hasVoted: expected %v, got %v", name, hasVoted, pp["hasVoted"])
			}
			if vote != nil {
				v := pp["vote"]
				if v == nil || v.(string) != *vote {
					t.Fatalf("participant %s vote: expected %s, got %v", name, *vote, v)
				}
			}
			return
		}
	}
	t.Fatalf("participant %s not found", name)
}

func extractParam(s, prefix, suffix string) string {
	start := strings.Index(s, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	if suffix == "" {
		return s[start:]
	}
	end := strings.Index(s[start:], suffix)
	if end < 0 {
		return s[start:]
	}
	return s[start : start+end]
}

func strPtr(s string) *string { return &s }

func TestNotFoundRedirectsHome(t *testing.T) {
	hub = &Hub{Rooms: make(map[string]*Room)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("GET /{path...}", handleNotFound)
	mux.HandleFunc("POST /create", handleCreateRoom)
	mux.HandleFunc("GET /room/{id}", handleRoomPage)

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 1. Unknown route redirects to home
	resp, err := client.Get(server.URL + "/whatever/path")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("unknown route: expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Fatalf("unknown route: expected redirect to /, got %s", loc)
	}

	// 2. Stale room redirects to home with error param
	resp, err = client.Get(server.URL + "/room/NOPE42")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("stale room: expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/?error=room_not_found" {
		t.Fatalf("stale room: expected redirect to /?error=room_not_found, got %s", loc)
	}

	// 3. Home renders the notice for a stale room
	resp, err = client.Get(server.URL + "/?error=room_not_found")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "existe plus") {
		t.Fatalf("home should show notice for stale room")
	}
	t.Logf("404 redirect behaviors OK")
}

func newTestRoom(t *testing.T, httpURL string) (string, string) {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Post(httpURL+"/create", "", nil)
	if err != nil {
		t.Fatal("create room:", err)
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	if location == "" {
		t.Fatal("no redirect after create")
	}
	return extractParam(location, "/room/", "?"), extractParam(location, "admin=", "")
}

func dialWS(t *testing.T, wsURL, roomID, name, admin string) *websocket.Conn {
	t.Helper()
	host := strings.TrimPrefix(wsURL, "ws://")
	raw := map[string]string{"name": name}
	vals := url.Values{}
	for k, v := range raw {
		vals.Set(k, v)
	}
	if admin != "" {
		vals.Set("admin", admin)
	}
	u := url.URL{Scheme: "ws", Host: host, Path: "/ws/" + roomID, RawQuery: vals.Encode()}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	return c
}

func assertParticipantAdmin(t *testing.T, data map[string]interface{}, name string, isAdmin bool) {
	t.Helper()
	parts := data["participants"].([]interface{})
	for _, p := range parts {
		pp := p.(map[string]interface{})
		if pp["name"] == name {
			if pp["isAdmin"] != isAdmin {
				t.Fatalf("participant %s isAdmin: expected %v, got %v", name, isAdmin, pp["isAdmin"])
			}
			return
		}
	}
	t.Fatalf("participant %s not found", name)
}

func assertNoParticipant(t *testing.T, data map[string]interface{}, name string) {
	t.Helper()
	parts := data["participants"].([]interface{})
	for _, p := range parts {
		if p.(map[string]interface{})["name"] == name {
			t.Fatalf("participant %s should have been removed", name)
		}
	}
}

func TestAdminManagement(t *testing.T) {
	hub = &Hub{Rooms: make(map[string]*Room)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", handleCreateRoom)
	mux.HandleFunc("GET /room/{id}", handleRoomPage)
	mux.HandleFunc("GET /ws/{id}", handleWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	roomID, adminToken := newTestRoom(t, server.URL)

	// Admin Alice + participants Bob (first) and Charlie
	admin := dialWS(t, wsURL, roomID, "Alice", adminToken)
	defer admin.Close()
	readWS(t, admin, "admin init")

	bob := dialWS(t, wsURL, roomID, "Bob", "")
	defer bob.Close()
	readWS(t, bob, "bob init")
	readWS(t, admin, "admin sees bob joined")       // user_joined
	readWS(t, admin, "admin state with bob")        // state

	charlie := dialWS(t, wsURL, roomID, "Charlie", "")
	defer charlie.Close()
	readWS(t, charlie, "charlie init")
	readWS(t, admin, "admin sees charlie joined")   // user_joined
	state := readWS(t, admin, "admin state with charlie")
	assertState(t, state, 3, false)

	// Bob disconnects
	bob.Close()
	readWS(t, admin, "admin sees bob leave")        // user_left
	state = readWS(t, admin, "admin state after bob leave")
	assertParticipant(t, state, "Bob", false, nil)
	readWS(t, charlie, "charlie sees bob leave")    // user_left
	readWS(t, charlie, "charlie state after bob leave")

	// Admin removes disconnected Bob
	writeWS(t, admin, map[string]interface{}{"type": "remove", "name": "Bob"})
	state = readWS(t, admin, "admin after remove")
	assertState(t, state, 2, false)
	assertNoParticipant(t, state, "Bob")
	readWS(t, charlie, "charlie after remove")
	t.Logf("Admin removed disconnected participant")

	// Admin promotes Charlie (transfer)
	writeWS(t, admin, map[string]interface{}{"type": "promote", "name": "Charlie"})
	state = readWS(t, admin, "admin after promote")
	assertParticipantAdmin(t, state, "Charlie", true)
	assertParticipantAdmin(t, state, "Alice", false)
	state = readWS(t, charlie, "charlie after promote")
	assertParticipantAdmin(t, state, "Charlie", true)
	t.Logf("Admin transferred to Charlie")

	// Charlie (new admin) can reveal
	writeWS(t, charlie, map[string]interface{}{"type": "reveal"})
	state = readWS(t, charlie, "charlie reveal")
	assertState(t, state, 2, true)

	// Alice (former admin) can no longer reset votes
	writeWS(t, admin, map[string]interface{}{"type": "reset"})
	writeWS(t, admin, map[string]interface{}{"type": "vote", "value": "8"}) // triggers a state push
	state = readWS(t, admin, "alice tries to reset")
	if state["revealed"] != true {
		t.Fatal("former admin managed to reset votes")
	}
	t.Logf("Alice no longer controls reset")
}

func TestAdminAutoTransferOnDisconnect(t *testing.T) {
	hub = &Hub{Rooms: make(map[string]*Room)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", handleCreateRoom)
	mux.HandleFunc("GET /room/{id}", handleRoomPage)
	mux.HandleFunc("GET /ws/{id}", handleWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	roomID, adminToken := newTestRoom(t, server.URL)

	admin := dialWS(t, wsURL, roomID, "Alice", adminToken)
	readWS(t, admin, "admin init")

	bob := dialWS(t, wsURL, roomID, "Bob", "")
	defer bob.Close()
	readWS(t, bob, "bob init")
	readWS(t, admin, "admin sees bob")  // user_joined
	readWS(t, admin, "admin state")     // state with 2

	// Admin disconnects -> Bob, first connected participant, becomes admin
	admin.Close()
	readWS(t, bob, "bob sees admin leave")  // user_left
	state := readWS(t, bob, "bob state after admin leave")
	assertState(t, state, 2, false)
	assertParticipantAdmin(t, state, "Bob", true)
	assertParticipantAdmin(t, state, "Alice", false)
	t.Logf("Bob auto-promoted to admin after admin disconnect")

	// Bob can now reveal
	writeWS(t, bob, map[string]interface{}{"type": "reveal"})
	state = readWS(t, bob, "bob reveals")
	assertState(t, state, 2, true)
	t.Logf("Auto-transferred admin can reveal")
}