package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type PageWriting struct {
	PageNumber int    `json:"page_number"`
	Heading    string `json:"heading"`
	Text       string `json:"text"`
}

func GeneratePageWriting(contextText, style string, plan MagazinePlan, analyses []PhotoAnalysis) ([]PageWriting, error) {
	model := strings.TrimSpace(os.Getenv("OPENROUTER_TEXT_MODEL"))
	if model == "" {
		return nil, fmt.Errorf("OPENROUTER_TEXT_MODEL is not set")
	}
	style = strings.ToLower(strings.TrimSpace(style))
	switch style {
	case "caption", "prose", "poem":
	default:
		return nil, fmt.Errorf("writing style must be caption, prose, or poem")
	}
	input, err := json.Marshal(struct {
		TripContext string          `json:"trip_context"`
		Style       string          `json:"writing_style"`
		Plan        MagazinePlan    `json:"layout_plan"`
		Photos      []PhotoAnalysis `json:"photo_analyses"`
	}{contextText, style, plan, analyses})
	if err != nil {
		return nil, fmt.Errorf("encode writing input: %w", err)
	}
	prompt := fmt.Sprintf(`Write independent editorial copy for the already-planned magazine pages. Do not change page order, layout IDs, or photo assignments. Use only trip context and visible photo analysis; never invent events, places, or personal details. Return JSON only: {"pages":[{"page_number":1,"heading":"short optional heading","text":"page copy"}]}. Return exactly one writing record for every page number in the plan. Writing style: %s. Keep every page's text at or below 260 characters, and every heading at or below 100 characters. Captions must be one concise sentence; prose must be 2–3 short sentences; poems must be 2–4 brief lines. Count characters before returning and shorten any text that exceeds the limit. Avoid generic filler.`, style)
	output, err := callOpenRouterChat(model, []chatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: string(input)},
	}, 2400)
	if err != nil {
		return nil, fmt.Errorf("generate page writing: %w", err)
	}
	var result struct {
		Pages []PageWriting `json:"pages"`
	}
	if err := decodeModelJSON(output, &result); err != nil {
		return nil, fmt.Errorf("parse page writing JSON: %w", err)
	}
	if err := ValidatePageWriting(result.Pages, len(plan.Pages)); err != nil {
		return nil, err
	}
	return result.Pages, nil
}

func ValidatePageWriting(pages []PageWriting, expectedPages int) error {
	if expectedPages < 1 || expectedPages > 20 {
		return fmt.Errorf("writing must contain between 1 and 20 pages")
	}
	if len(pages) != expectedPages {
		return fmt.Errorf("writing contains %d pages; expected %d", len(pages), expectedPages)
	}
	seen := make(map[int]bool, expectedPages)
	for index, page := range pages {
		if page.PageNumber != index+1 || page.PageNumber < 1 || page.PageNumber > expectedPages || seen[page.PageNumber] {
			return fmt.Errorf("writing contains an invalid or duplicate page number %d", page.PageNumber)
		}
		textLength := len([]rune(strings.TrimSpace(page.Text)))
		headingLength := len([]rune(strings.TrimSpace(page.Heading)))
		if textLength == 0 {
			return fmt.Errorf("writing for page %d is empty", page.PageNumber)
		}
		if textLength > 300 {
			return fmt.Errorf("writing for page %d exceeds the 300-character limit (%d characters)", page.PageNumber, textLength)
		}
		if headingLength > 120 {
			return fmt.Errorf("heading for page %d exceeds the 120-character limit (%d characters)", page.PageNumber, headingLength)
		}
		seen[page.PageNumber] = true
	}
	return nil
}
