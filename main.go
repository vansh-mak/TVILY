package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/vansh-mak/tvily/handlers"
)

//go:embed web
var webFiles embed.FS

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found")
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/upload", handlers.UploadHandler)
	http.HandleFunc("POST /generate/{job_id}", handlers.GenerateHandler)
	http.HandleFunc("POST /render/{job_id}", handlers.RenderPDFHandler)
	web, err := fs.Sub(webFiles, "web")
	if err != nil {
		fmt.Println("Could not load web interface:", err)
		return
	}
	http.Handle("/", http.FileServer(http.FS(web)))

	fmt.Println("TVILY server running on http://localhost:8080")

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Server error:", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("TVILY is alive"))
}
