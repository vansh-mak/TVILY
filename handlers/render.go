package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/vansh-mak/tvily/ai"
	"github.com/vansh-mak/tvily/pdf"
	"github.com/vansh-mak/tvily/renderer"
)

func RenderPDFHandler(w http.ResponseWriter, r *http.Request) {
	jobID, ok := parseJobID(w, r)
	if !ok {
		return
	}
	tempDir := filepath.Join("temp", jobID)
	preparedData, err := os.ReadFile(filepath.Join(tempDir, "prepared_magazine.json"))
	if err != nil {
		http.Error(w, "Magazine plan not found; create the magazine draft first", http.StatusNotFound)
		return
	}
	var prepared preparedMagazine
	if err := json.Unmarshal(preparedData, &prepared); err != nil {
		http.Error(w, "Could not read the saved magazine plan", http.StatusInternalServerError)
		return
	}
	var writing []ai.PageWriting
	if writingData, readErr := os.ReadFile(filepath.Join(tempDir, "page_writing.json")); readErr == nil {
		if err := json.Unmarshal(writingData, &writing); err != nil {
			http.Error(w, "Could not read the saved page writing", http.StatusInternalServerError)
			return
		}
		if err := ai.ValidatePageWriting(writing, len(prepared.Plan.Pages)); err != nil {
			http.Error(w, "Saved page writing is invalid: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if !os.IsNotExist(readErr) {
		http.Error(w, "Could not read saved page writing", http.StatusInternalServerError)
		return
	}
	if err := rebuildJobPDF(jobID, tempDir, prepared, writing); err != nil {
		detail := redactProviderToken(err.Error())
		log.Printf("TVILY PDF rebuild failed for job %s: %s", jobID, detail)
		http.Error(w, "Could not rebuild the PDF. Check the page backgrounds and uploaded photos, then try again. Details: "+detail, http.StatusBadGateway)
		return
	}
	serveJobPDF(w, r, jobID, tempDir)
}

func rebuildJobPDF(jobID, tempDir string, prepared preparedMagazine, writing []ai.PageWriting) error {
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("read job files: %w", err)
	}
	photoPathByID := make(map[string]string)
	photoIDs := make([]string, 0)
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || name == "output.png" || name == "output.pdf" || name == "theme_background.png" || name == "prepared_magazine.json" || name == "page_writing.json" || strings.HasPrefix(name, "page_") || strings.HasPrefix(name, "background_page_") {
			continue
		}
		photoPathByID[name] = filepath.Join(tempDir, name)
		photoIDs = append(photoIDs, name)
	}
	catalog, err := renderer.LoadTemplateCatalog("templates/templates.json")
	if err != nil {
		return fmt.Errorf("load template catalog: %w", err)
	}
	pagePhotoCounts := make([]int, len(prepared.Plan.Pages))
	for index, page := range prepared.Plan.Pages {
		pagePhotoCounts[index] = len(page.PhotoIDs)
	}
	var availableLayouts []ai.PlannerLayout
	for _, item := range catalog.Templates {
		count, countErr := renderer.TemplatePhotoCount(catalog, item.ID)
		if countErr == nil {
			availableLayouts = append(availableLayouts, ai.PlannerLayout{ID: item.ID, Name: item.Name, PhotoCount: count})
		}
	}
	if err := ai.ValidateMagazinePlan(prepared.Plan, photoIDs, pagePhotoCounts, availableLayouts); err != nil {
		return fmt.Errorf("validate saved page plan: %w", err)
	}
	backgroundPaths := make([]string, len(prepared.Plan.Pages))
	backgroundsByDirection := make(map[string]string)
	for index, page := range prepared.Plan.Pages {
		direction := strings.TrimSpace(page.BackgroundDirection)
		if direction == "" {
			direction = prepared.TripContext
		}
		backgroundPath := filepath.Join(tempDir, fmt.Sprintf("background_page_%02d.png", index+1))
		if _, statErr := os.Stat(backgroundPath); statErr != nil {
			if priorPath := backgroundsByDirection[direction]; priorPath != "" {
				generated, readErr := os.ReadFile(priorPath)
				if readErr != nil {
					return fmt.Errorf("reuse page background: %w", readErr)
				}
				if writeErr := os.WriteFile(backgroundPath, generated, 0600); writeErr != nil {
					return fmt.Errorf("save repeated page background: %w", writeErr)
				}
			} else if err := ai.GenerateThemeBackground(prepared.TripContext, direction, backgroundPath); err != nil {
				return fmt.Errorf("generate page %d background: %w", index+1, err)
			}
		}
		backgroundsByDirection[direction] = backgroundPath
		backgroundPaths[index] = backgroundPath
	}
	writingByPage := make(map[int]ai.PageWriting, len(writing))
	for _, page := range writing {
		writingByPage[page.PageNumber] = page
	}
	pagePaths := make([]string, 0, len(prepared.Plan.Pages))
	for index, page := range prepared.Plan.Pages {
		pagePhotos := make([]string, 0, len(page.PhotoIDs))
		for _, photoID := range page.PhotoIDs {
			path := photoPathByID[photoID]
			if path == "" {
				return fmt.Errorf("photo %q is missing from the job", photoID)
			}
			pagePhotos = append(pagePhotos, path)
		}
		copy := writingByPage[page.PageNumber]
		pageText := renderer.PageText{Heading: copy.Heading, Text: copy.Text}
		pagePath := filepath.Join(tempDir, fmt.Sprintf("page_%02d.png", index+1))
		if err := renderer.RenderTemplateAutoWithText(catalog, page.LayoutID, pagePhotos, pagePath, pageText, backgroundPaths[index]); err != nil {
			return fmt.Errorf("render page %d: %w", index+1, err)
		}
		pagePaths = append(pagePaths, pagePath)
	}
	if err := pdf.CreateFromPNGs(pagePaths, filepath.Join(tempDir, "output.pdf")); err != nil {
		return fmt.Errorf("create PDF: %w", err)
	}
	return nil
}

func serveJobPDF(w http.ResponseWriter, r *http.Request, jobID, tempDir string) {
	output, err := os.Open(filepath.Join(tempDir, "output.pdf"))
	if err != nil {
		http.Error(w, "Could not open generated PDF", http.StatusInternalServerError)
		return
	}
	defer output.Close()
	info, err := output.Stat()
	if err != nil {
		http.Error(w, "Could not read generated PDF", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="tvily-%s.pdf"`, jobID))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "tvily-magazine.pdf", info.ModTime(), output)
}
