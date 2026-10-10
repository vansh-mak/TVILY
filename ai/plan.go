package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MagazinePlan is the validated, structured editorial plan used to render a magazine.
type MagazinePlan struct {
	Title string        `json:"title"`
	Style string        `json:"style"`
	Pages []PlannedPage `json:"pages"`
}

type PlannedPage struct {
	PageNumber          int      `json:"page_number"`
	LayoutID            string   `json:"layout_id"`
	PhotoIDs            []string `json:"photo_ids"`
	BackgroundDirection string   `json:"background_direction"`
	WritingDirection    string   `json:"writing_direction"`
}

type PlannerLayout struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PhotoCount int    `json:"photo_count"`
}

func GenerateMagazinePlan(contextText string, photoIDs []string, pagePhotoCounts []int, photoAnalyses []PhotoAnalysis, layouts []PlannerLayout) (MagazinePlan, error) {
	contextText = strings.TrimSpace(contextText)
	if contextText == "" {
		return MagazinePlan{}, fmt.Errorf("trip context is required")
	}
	if len(contextText) > 3000 {
		return MagazinePlan{}, fmt.Errorf("trip context must be 3000 characters or fewer")
	}
	model := strings.TrimSpace(os.Getenv("OPENROUTER_TEXT_MODEL"))
	if model == "" {
		return MagazinePlan{}, fmt.Errorf("OPENROUTER_TEXT_MODEL is not set")
	}

	userInput := struct {
		Context         string          `json:"trip_context"`
		PhotoAnalyses   []PhotoAnalysis `json:"photos"`
		PagePhotoCounts []int           `json:"page_photo_counts"`
		Layouts         []PlannerLayout `json:"available_layouts"`
	}{contextText, photoAnalyses, pagePhotoCounts, layouts}
	inputJSON, err := json.Marshal(userInput)
	if err != nil {
		return MagazinePlan{}, fmt.Errorf("encode planner input: %w", err)
	}
	modelOutput, err := callOpenRouterChat(model, []chatMessage{
		{Role: "system", Content: `Create a photo magazine page plan. Return only a JSON object with this shape:
{"title":"short magazine title","style":"short visual style","pages":[{"page_number":1,"layout_id":"existing layout id","photo_ids":["photo_01.jpg"],"background_direction":"page-specific atmospheric art direction consistent with the magazine theme","writing_direction":"brief page-specific prose direction"}]}
	Group related photos into narrative pages using their descriptions and the trip context. The page_photo_counts array gives the exact number of photos for each page; use each supplied photo ID exactly once across the whole magazine. Select layouts whose photo_count exactly matches each page. Never invent layout IDs or photo IDs. Keep directions concise and do not claim events absent from the context or image descriptions.`},
		{Role: "user", Content: string(inputJSON)},
	}, 6000)
	if err != nil {
		return MagazinePlan{}, fmt.Errorf("generate magazine plan: %w", err)
	}
	var plan MagazinePlan
	if err := decodeModelJSON(modelOutput, &plan); err != nil {
		return MagazinePlan{}, fmt.Errorf("parse magazine plan JSON: %w", err)
	}
	if err := ValidateMagazinePlan(plan, photoIDs, pagePhotoCounts, layouts); err != nil {
		return MagazinePlan{}, err
	}
	return plan, nil
}

func ValidateMagazinePlan(plan MagazinePlan, photoIDs []string, pagePhotoCounts []int, layouts []PlannerLayout) error {
	if len(pagePhotoCounts) < 1 || len(pagePhotoCounts) > 20 {
		return fmt.Errorf("magazine plan must contain between 1 and 20 pages")
	}
	if strings.TrimSpace(plan.Title) == "" || strings.TrimSpace(plan.Style) == "" {
		return fmt.Errorf("magazine plan is missing its title or style")
	}
	if len([]rune(plan.Title)) > 120 || len([]rune(plan.Style)) > 200 {
		return fmt.Errorf("magazine plan title or style exceeds the allowed length")
	}
	if len(plan.Pages) != len(pagePhotoCounts) {
		return fmt.Errorf("magazine plan has %d pages; expected %d", len(plan.Pages), len(pagePhotoCounts))
	}
	knownPhotos := make(map[string]bool, len(photoIDs))
	for _, id := range photoIDs {
		if id == "" || knownPhotos[id] {
			return fmt.Errorf("uploaded photo IDs must be non-empty and unique")
		}
		knownPhotos[id] = true
	}
	knownLayouts := make(map[string]int, len(layouts))
	for _, layout := range layouts {
		knownLayouts[layout.ID] = layout.PhotoCount
	}
	seen := make(map[string]bool, len(photoIDs))
	for index, page := range plan.Pages {
		if pagePhotoCounts[index] < 2 || pagePhotoCounts[index] > 6 {
			return fmt.Errorf("page %d must contain between 2 and 6 photos", index+1)
		}
		if page.PageNumber != index+1 {
			return fmt.Errorf("page %d has invalid page_number %d", index+1, page.PageNumber)
		}
		if strings.TrimSpace(page.BackgroundDirection) == "" || strings.TrimSpace(page.WritingDirection) == "" {
			return fmt.Errorf("page %d is missing a background or writing direction", index+1)
		}
		if len([]rune(page.BackgroundDirection)) > 500 || len([]rune(page.WritingDirection)) > 500 {
			return fmt.Errorf("page %d directions exceed the allowed length", index+1)
		}
		layoutPhotoCount, ok := knownLayouts[page.LayoutID]
		if !ok {
			return fmt.Errorf("page %d uses unknown layout %q", index+1, page.LayoutID)
		}
		if layoutPhotoCount != len(page.PhotoIDs) || len(page.PhotoIDs) != pagePhotoCounts[index] {
			return fmt.Errorf("page %d layout/photo count does not match its assignment", index+1)
		}
		for _, id := range page.PhotoIDs {
			if !knownPhotos[id] || seen[id] {
				return fmt.Errorf("page %d has an invalid or repeated photo ID %q", index+1, id)
			}
			seen[id] = true
		}
	}
	if len(seen) != len(photoIDs) {
		return fmt.Errorf("magazine plan does not use every uploaded photo exactly once")
	}
	return nil
}
