package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type pageData struct {
	ID         string
	AdminToken string
	IsAdmin    bool
	RoomName   string
	Fibonacci  []string
}

type indexData struct {
	Error string
	Rooms []RoomSummary
}

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS))))

	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("GET /{path...}", handleNotFound)
	mux.HandleFunc("POST /create", handleCreateRoom)
	mux.HandleFunc("GET /room/{id}", handleRoomPage)
	mux.HandleFunc("GET /ws/{id}", handleWebSocket)

	log.Printf("Serveur démarré sur :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	data := indexData{
		Rooms: hub.ListRooms(),
	}
	if q := r.URL.Query().Get("error"); q == "room_not_found" {
		data.Error = "Ce salon n'existe plus. Pensez à en créer un nouveau."
	}
	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Println("template error:", err)
	}
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}