package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	// Bound the complete multipart request, including headers and form fields.
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "Could not parse upload; keep the total upload under 50 MB", http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["photos"]

	if len(files) == 0 {
		http.Error(w, "No photos uploaded", http.StatusBadRequest)
		return
	}

	if len(files) > 20 {
		http.Error(w, "Maximum 20 photos allowed", http.StatusBadRequest)
		return
	}

	// Create a unique ID for this magazine generation.
	jobID := uuid.New().String()

	// Each upload gets its own temporary folder.
	tempDir := filepath.Join("temp", jobID)

	err = os.MkdirAll(tempDir, 0755)
	if err != nil {
		http.Error(w, "Could not create temp directory", http.StatusInternalServerError)
		return
	}

	// Save every uploaded photo.
	for index, fileHeader := range files {
		extension := strings.ToLower(filepath.Ext(filepath.Base(fileHeader.Filename)))
		switch extension {
		case ".jpg", ".jpeg", ".png", ".webp":
		default:
			_ = os.RemoveAll(tempDir)
			http.Error(w, "Photos must be JPEG, PNG, or WebP files", http.StatusBadRequest)
			return
		}
		if fileHeader.Size <= 0 {
			_ = os.RemoveAll(tempDir)
			http.Error(w, "Uploaded photo is empty", http.StatusBadRequest)
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			_ = os.RemoveAll(tempDir)
			http.Error(w, "Could not open uploaded file", http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("photo_%02d%s", index+1, filepath.Ext(filepath.Base(fileHeader.Filename)))
		destination := filepath.Join(tempDir, filename)

		out, err := os.Create(destination)
		if err != nil {
			file.Close()
			_ = os.RemoveAll(tempDir)
			http.Error(w, "Could not save file", http.StatusInternalServerError)
			return
		}

		_, err = io.Copy(out, file)

		file.Close()
		out.Close()

		if err != nil {
			_ = os.RemoveAll(tempDir)
			http.Error(w, "Could not save file", http.StatusInternalServerError)
			return
		}

		fmt.Println("Uploaded:", filename)
	}

	// Return the temporary job ID to the client.
	w.Header().Set("Content-Type", "application/json")

	fmt.Fprintf(w, `{
		"job_id": "%s",
		"message": "Photos uploaded successfully"
	}`, jobID)
}
