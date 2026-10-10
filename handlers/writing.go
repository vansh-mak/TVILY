package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/vansh-mak/tvily/ai"
)

type preparedMagazine struct {
	TripContext   string             `json:"trip_context"`
	Plan          ai.MagazinePlan    `json:"plan"`
	PhotoAnalyses []ai.PhotoAnalysis `json:"photo_analyses"`
}

type pageWritingRequest struct {
	Style string           `json:"style"`
	Pages []ai.PageWriting `json:"pages"`
}

func WritingHandler(w http.ResponseWriter, r *http.Request) {
	jobID, ok := parseJobID(w, r)
	if !ok {
		return
	}
	tempDir := filepath.Join("temp", jobID)
	preparedData, err := os.ReadFile(filepath.Join(tempDir, "prepared_magazine.json"))
	if err != nil {
		http.Error(w, "Magazine plan not found; generate the PDF draft first", http.StatusNotFound)
		return
	}
	var prepared preparedMagazine
	if err := json.Unmarshal(preparedData, &prepared); err != nil {
		http.Error(w, "Could not read the saved magazine plan", http.StatusInternalServerError)
		return
	}
	var input pageWritingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil {
		http.Error(w, "Invalid page writing request", http.StatusBadRequest)
		return
	}
	var pages []ai.PageWriting
	if input.Pages != nil {
		if err := ai.ValidatePageWriting(input.Pages, len(prepared.Plan.Pages)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		pages = input.Pages
	} else {
		saved, readErr := os.ReadFile(filepath.Join(tempDir, "page_writing.json"))
		if readErr == nil {
			if err := json.Unmarshal(saved, &pages); err != nil {
				http.Error(w, "Could not read saved page writing", http.StatusInternalServerError)
				return
			}
		} else {
			var err error
			pages, err = ai.GeneratePageWriting(prepared.TripContext, input.Style, prepared.Plan, prepared.PhotoAnalyses)
			if err != nil {
				detail := redactProviderToken(err.Error())
				http.Error(w, "Could not write page copy. Check OPENROUTER_API_KEY and OPENROUTER_TEXT_MODEL, then try again. Details: "+detail, http.StatusBadGateway)
				return
			}
		}
	}
	encoded, err := json.MarshalIndent(pages, "", "  ")
	if err != nil {
		http.Error(w, "Could not encode page writing", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(tempDir, "page_writing.json"), encoded, 0600); err != nil {
		http.Error(w, "Could not save page writing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Pages []ai.PageWriting `json:"pages"`
	}{pages})
}

func parseJobID(w http.ResponseWriter, r *http.Request) (string, bool) {
	parsed, err := uuid.Parse(r.PathValue("job_id"))
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return "", false
	}
	return parsed.String(), true
}
