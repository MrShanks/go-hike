package main

import (
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
)

type server struct {
	dataDir string
	static  fs.FS
}

func main() {
	static, err := fs.Sub(assets, "web")
	if err != nil {
		log.Fatal(err)
	}

	app := &server{dataDir: "data", static: static}
	if err := os.MkdirAll(app.dataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	address := ":8080"
	log.Printf("Tracks is running at http://localhost%s", address)
	log.Fatal(http.ListenAndServe(address, app.routes()))
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tracks", s.listTracks)
	mux.HandleFunc("POST /api/tracks", s.importTracks)
	mux.HandleFunc("PATCH /api/tracks/{id}", s.renameTrack)
	mux.HandleFunc("DELETE /api/tracks/{id}", s.deleteTrack)
	mux.HandleFunc("POST /api/tracks/{id}/photos", s.importPhotos)
	mux.HandleFunc("GET /api/photos", s.listPhotos)
	mux.HandleFunc("POST /api/photos", s.importPhotos)
	mux.HandleFunc("GET /api/photos/{id}/image", s.servePhoto)
	mux.HandleFunc("DELETE /api/photos/{id}", s.deletePhoto)
	mux.Handle("/", http.FileServer(http.FS(s.static)))
	return mux
}

func validID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func multipartFieldNames(files map[string][]*multipart.FileHeader) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
