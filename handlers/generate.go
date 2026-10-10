package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vansh-mak/tvily/ai"
	"github.com/vansh-mak/tvily/renderer"
)

func GenerateHandler(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("job_id")

	parsedJobID, err := uuid.Parse(jobID)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	jobID = parsedJobID.String()

	tempDir := filepath.Join("temp", jobID)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	if len(files) < 2 || len(files) > 20 {
		http.Error(w, "Upload between 2 and 20 photos", http.StatusBadRequest)
		return
	}

	// Collect uploaded photos and ignore outputs from earlier generations.
	var photoPaths []string

	for _, file := range files {
		if file.IsDir() || file.Name() == "output.png" || file.Name() == "output.pdf" || file.Name() == "theme_background.png" || file.Name() == "prepared_magazine.json" || file.Name() == "page_writing.json" || strings.HasPrefix(file.Name(), "page_") || strings.HasPrefix(file.Name(), "background_page_") {
			continue
		}

		photoPath := filepath.Join(tempDir, file.Name())
		photoPaths = append(photoPaths, photoPath)
	}

	catalog, err := renderer.LoadTemplateCatalog("templates/templates.json")
	if err != nil {
		fmt.Println("Template catalog error:", err)
		http.Error(w, "Could not load layouts", http.StatusInternalServerError)
		return
	}
	contextText := strings.TrimSpace(r.URL.Query().Get("trip_context"))
	if contextText == "" { // Keep compatibility with older clients.
		contextText = strings.TrimSpace(r.URL.Query().Get("theme"))
	}
	if contextText == "" {
		http.Error(w, "Enter trip context", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(contextText) > 3000 {
		http.Error(w, "Trip context must be 3000 characters or fewer", http.StatusBadRequest)
		return
	}

	var plan ai.MagazinePlan
	requestedLayouts := r.URL.Query()["layout"]
	if len(requestedLayouts) > 0 {
		if len(requestedLayouts) > 20 {
			http.Error(w, "Maximum 20 pages per PDF", http.StatusBadRequest)
			return
		}
		plan = ai.MagazinePlan{Title: "TVILY Magazine", Style: "User-selected layouts", Pages: make([]ai.PlannedPage, 0, len(requestedLayouts))}
		photoOffset := 0
		for pageIndex, layoutID := range requestedLayouts {
			count, countErr := renderer.TemplatePhotoCount(catalog, layoutID)
			if countErr != nil {
				http.Error(w, "Unknown layout: "+layoutID, http.StatusBadRequest)
				return
			}
			if photoOffset+count > len(photoPaths) {
				http.Error(w, "Selected layouts require more photos than uploaded", http.StatusBadRequest)
				return
			}
			ids := make([]string, 0, count)
			for _, photoPath := range photoPaths[photoOffset : photoOffset+count] {
				ids = append(ids, filepath.Base(photoPath))
			}
			plan.Pages = append(plan.Pages, ai.PlannedPage{PageNumber: pageIndex + 1, LayoutID: layoutID, PhotoIDs: ids, BackgroundDirection: contextText, WritingDirection: "No page writing generated."})
			photoOffset += count
		}
		if photoOffset != len(photoPaths) {
			http.Error(w, fmt.Sprintf("Selected layouts require exactly %d photos", photoOffset), http.StatusBadRequest)
			return
		}
	} else {
		if len(photoPaths) < 2 || len(photoPaths) > 20 {
			http.Error(w, "Upload between 2 and 20 photos", http.StatusBadRequest)
			return
		}
		pagePhotoCounts, err := requestedPagePhotoCounts(r, len(photoPaths))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		layoutIDsByCount := make(map[int][]string)
		for _, item := range catalog.Templates {
			count, countErr := renderer.TemplatePhotoCount(catalog, item.ID)
			if countErr != nil {
				continue
			}
			layoutIDsByCount[count] = append(layoutIDsByCount[count], item.ID)
		}
		plan = ai.MagazinePlan{Title: "TVILY Magazine", Style: "Photo-led layouts", Pages: make([]ai.PlannedPage, 0, len(pagePhotoCounts))}
		photoOffset := 0
		layoutIndexes := make(map[int]int)
		for pageIndex, count := range pagePhotoCounts {
			choices := layoutIDsByCount[count]
			if len(choices) == 0 {
				http.Error(w, fmt.Sprintf("No layout is available for %d photos per page", count), http.StatusInternalServerError)
				return
			}
			layoutIndex := layoutIndexes[count] % len(choices)
			layoutIndexes[count]++
			ids := make([]string, 0, count)
			for _, photoPath := range photoPaths[photoOffset : photoOffset+count] {
				ids = append(ids, filepath.Base(photoPath))
			}
			plan.Pages = append(plan.Pages, ai.PlannedPage{
				PageNumber: pageIndex + 1, LayoutID: choices[layoutIndex], PhotoIDs: ids,
				BackgroundDirection: contextText, WritingDirection: "No page writing generated.",
			})
			photoOffset += count
		}
	}
	prepared, err := json.MarshalIndent(preparedMagazine{TripContext: contextText, Plan: plan}, "", "  ")
	if err != nil {
		http.Error(w, "Could not encode the magazine plan", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(tempDir, "prepared_magazine.json"), prepared, 0600); err != nil {
		http.Error(w, "Could not save the magazine plan", http.StatusInternalServerError)
		return
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "background_page_") {
			_ = os.Remove(filepath.Join(tempDir, file.Name()))
		}
	}
	// Planning is local and deterministic; only page backgrounds use a model.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Plan ai.MagazinePlan `json:"plan"`
	}{Plan: plan})
}

func requestedPagePhotoCounts(r *http.Request, photoCount int) ([]int, error) {
	pageCount := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("page_count")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 20 {
			return nil, fmt.Errorf("Page count must be between 1 and 20")
		}
		pageCount = parsed
	}
	target := 4
	if raw := strings.TrimSpace(r.URL.Query().Get("photos_per_page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 2 || parsed > 6 {
			return nil, fmt.Errorf("Photos per page must be between 2 and 6")
		}
		target = parsed
	}
	if pageCount == 0 {
		pageCount = int(math.Round(float64(photoCount) / float64(target)))
		minimumPages := (photoCount + 5) / 6
		maximumPages := photoCount / 2
		if pageCount < minimumPages {
			pageCount = minimumPages
		}
		if pageCount > maximumPages {
			pageCount = maximumPages
		}
	}
	if pageCount > photoCount/2 || pageCount*6 < photoCount {
		return nil, fmt.Errorf("The selected page count cannot fit all photos; each page needs 2 to 6 photos")
	}
	counts := make([]int, pageCount)
	base, extra := photoCount/pageCount, photoCount%pageCount
	for index := range counts {
		counts[index] = base
		if index < extra {
			counts[index]++
		}
	}
	return counts, nil
}
