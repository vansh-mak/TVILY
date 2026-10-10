package ai

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/disintegration/imaging"
)

type PhotoAnalysis struct {
	PhotoID     string `json:"photo_id"`
	Description string `json:"description"`
	Subject     string `json:"subject"`
	Setting     string `json:"setting"`
	VisualTone  string `json:"visual_tone"`
	Orientation string `json:"orientation"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

type imageMessagePart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *imageURLDetail `json:"image_url,omitempty"`
}

type imageURLDetail struct {
	URL string `json:"url"`
}

func AnalyzePhotos(photoIDs []string, photoPathByID map[string]string) ([]PhotoAnalysis, error) {
	model := strings.TrimSpace(os.Getenv("OPENROUTER_TEXT_MODEL"))
	if model == "" {
		return nil, fmt.Errorf("OPENROUTER_TEXT_MODEL is not set")
	}
	parts := make([]imageMessagePart, 0, len(photoIDs)*2+1)
	parts = append(parts, imageMessagePart{Type: "text", Text: `Analyze the supplied travel photos for magazine planning. Return only JSON in this shape:
{"photos":[{"photo_id":"photo_01.jpg","description":"one concise factual sentence","subject":"main visible subject","setting":"visible location or environment","visual_tone":"observable colors/light/mood"}]}
Use only the supplied photo IDs. Describe visible content; do not identify people, infer sensitive traits, or invent context. Keep each field concise.`})
	dimensions := make(map[string]image.Point, len(photoIDs))
	for _, photoID := range photoIDs {
		path := photoPathByID[photoID]
		if path == "" {
			return nil, fmt.Errorf("missing uploaded photo %q", photoID)
		}
		photo, err := imaging.Open(path, imaging.AutoOrientation(true))
		if err != nil {
			return nil, fmt.Errorf("open photo %s for analysis: %w", photoID, err)
		}
		bounds := photo.Bounds()
		dimensions[photoID] = image.Pt(bounds.Dx(), bounds.Dy())
		thumbnail := imaging.Fit(photo, 512, 512, imaging.Lanczos)
		var encoded bytes.Buffer
		if err := imaging.Encode(&encoded, thumbnail, imaging.JPEG, imaging.JPEGQuality(76)); err != nil {
			return nil, fmt.Errorf("prepare photo %s for analysis: %w", photoID, err)
		}
		parts = append(parts, imageMessagePart{Type: "text", Text: "Photo ID: " + photoID})
		parts = append(parts, imageMessagePart{Type: "image_url", ImageURL: &imageURLDetail{URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())}})
	}
	modelOutput, err := callOpenRouterChat(model, []chatMessage{
		{Role: "system", Content: "You are a careful visual analyst. Return valid JSON only."},
		{Role: "user", Content: parts},
	}, 2500)
	if err != nil {
		return nil, fmt.Errorf("analyze uploaded photos: %w", err)
	}
	var result struct {
		Photos []PhotoAnalysis `json:"photos"`
	}
	if err := decodeModelJSON(modelOutput, &result); err != nil {
		return nil, fmt.Errorf("parse photo analysis JSON: %w", err)
	}
	if len(result.Photos) != len(photoIDs) {
		return nil, fmt.Errorf("photo analysis returned %d records; expected %d", len(result.Photos), len(photoIDs))
	}
	known := make(map[string]bool, len(photoIDs))
	for _, id := range photoIDs {
		known[id] = true
	}
	seen := make(map[string]bool, len(photoIDs))
	for index := range result.Photos {
		photo := &result.Photos[index]
		if !known[photo.PhotoID] || seen[photo.PhotoID] {
			return nil, fmt.Errorf("photo analysis returned an unknown or duplicate photo ID %q", photo.PhotoID)
		}
		seen[photo.PhotoID] = true
		if strings.TrimSpace(photo.Description) == "" {
			return nil, fmt.Errorf("photo analysis omitted a description for %q", photo.PhotoID)
		}
		dim := dimensions[photo.PhotoID]
		photo.Width, photo.Height = dim.X, dim.Y
		switch {
		case dim.X > dim.Y:
			photo.Orientation = "landscape"
		case dim.X < dim.Y:
			photo.Orientation = "portrait"
		default:
			photo.Orientation = "square"
		}
	}
	return result.Photos, nil
}
