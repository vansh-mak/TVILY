package renderer

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/disintegration/imaging"
)

type Template struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Canvas Canvas `json:"canvas"`
	Slots  []Slot `json:"slots"`
}

type Canvas struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Background string `json:"background"`
}

type Slot struct {
	ID       string  `json:"id"`
	Role     string  `json:"role"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Rotation float64 `json:"rotation"`
	Padding  int     `json:"padding"`
}

type PageText struct {
	Heading string
	Text    string
}

func Render(
	templatePath string,
	photoPaths []string,
	assignments map[string]int,
	outputPath string,
	backgroundImagePath ...string,
) error {
	return render(templatePath, photoPaths, assignments, outputPath, backgroundImagePath, nil)
}

func RenderWithText(
	templatePath string,
	photoPaths []string,
	assignments map[string]int,
	outputPath string,
	pageText PageText,
	backgroundImagePath ...string,
) error {
	return render(templatePath, photoPaths, assignments, outputPath, backgroundImagePath, &pageText)
}

func render(
	templatePath string,
	photoPaths []string,
	assignments map[string]int,
	outputPath string,
	backgroundImagePath []string,
	pageText *PageText,
) error {
	// 1. Load template
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	var template Template

	err = json.Unmarshal(templateData, &template)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// 2. Create canvas
	canvas := image.NewRGBA(
		image.Rect(
			0,
			0,
			template.Canvas.Width,
			template.Canvas.Height,
		),
	)

	background, err := parseHexColor(template.Canvas.Background)
	if err != nil {
		return fmt.Errorf("parse canvas background: %w", err)
	}

	// 3. Fill canvas with the template background (white when unspecified).
	draw.Draw(
		canvas,
		canvas.Bounds(),
		&image.Uniform{C: background},
		image.Point{},
		draw.Src,
	)
	if len(backgroundImagePath) > 1 {
		return fmt.Errorf("at most one background image may be provided")
	}
	if len(backgroundImagePath) == 1 && backgroundImagePath[0] != "" {
		backgroundImage, err := imaging.Open(backgroundImagePath[0])
		if err != nil {
			return fmt.Errorf("open background image: %w", err)
		}
		backgroundImage = imaging.Fill(backgroundImage, template.Canvas.Width, template.Canvas.Height, imaging.Center, imaging.Lanczos)
		draw.Draw(canvas, canvas.Bounds(), backgroundImage, backgroundImage.Bounds().Min, draw.Src)
	}

	// 4. Resize each photo within its target bounds and place it on the canvas.
	photoScale := 1.0
	photoOffsetX, photoOffsetY := 0.0, 0.0
	if pageText != nil {
		photoRegionHeight := float64(template.Canvas.Height) * 0.78
		photoScale = math.Min(1, photoRegionHeight/float64(template.Canvas.Height))
		photoOffsetX = (float64(template.Canvas.Width) - float64(template.Canvas.Width)*photoScale) / 2
		photoOffsetY = (photoRegionHeight - float64(template.Canvas.Height)*photoScale) / 2
	}
	for _, slot := range template.Slots {

		photoIndex, exists := assignments[slot.ID]

		if !exists {
			continue
		}

		if photoIndex < 0 || photoIndex >= len(photoPaths) {
			return fmt.Errorf(
				"invalid photo index %d for slot %s",
				photoIndex,
				slot.ID,
			)
		}

		photo, err := imaging.Open(photoPaths[photoIndex], imaging.AutoOrientation(true))
		if err != nil {
			return fmt.Errorf(
				"open photo %s: %w",
				photoPaths[photoIndex],
				err,
			)
		}

		if slot.Width <= 0 || slot.Height <= 0 {
			return fmt.Errorf("invalid target dimensions for slot %s", slot.ID)
		}
		slotWidth := int(math.Round(float64(slot.Width) * photoScale))
		slotHeight := int(math.Round(float64(slot.Height) * photoScale))
		if slotWidth < 1 || slotHeight < 1 {
			return fmt.Errorf("scaled target dimensions are invalid for slot %s", slot.ID)
		}
		placed := image.Image(imaging.Fit(photo, slotWidth, slotHeight, imaging.Lanczos))
		if slot.Rotation != 0 {
			placed = imaging.Rotate(placed, slot.Rotation, color.Transparent)
		}
		placedX := int(math.Round(photoOffsetX + float64(slot.X)*photoScale))
		placedY := int(math.Round(photoOffsetY + float64(slot.Y)*photoScale))
		placedRect := image.Rect(placedX, placedY, placedX+placed.Bounds().Dx(), placedY+placed.Bounds().Dy())
		draw.Draw(canvas, placedRect, placed, placed.Bounds().Min, draw.Over)
	}
	if pageText != nil {
		if err := drawPageText(canvas, *pageText); err != nil {
			return err
		}
	}

	// 5. Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outputFile.Close()

	// 6. Encode as PNG
	err = png.Encode(outputFile, canvas)
	if err != nil {
		return fmt.Errorf("encode output: %w", err)
	}

	return nil
}

func parseHexColor(value string) (color.Color, error) {
	if value == "" {
		return color.White, nil
	}
	if len(value) != 7 || value[0] != '#' {
		return nil, fmt.Errorf("expected #RRGGBB, got %q", value)
	}
	components := make([]uint8, 3)
	for index := range components {
		component, err := strconv.ParseUint(value[1+index*2:3+index*2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid color %q: %w", value, err)
		}
		components[index] = uint8(component)
	}
	return color.RGBA{R: components[0], G: components[1], B: components[2], A: 255}, nil
}

func RenderTemplate(
	catalog TemplateCatalog,
	templateID string,
	photoPaths []string,
	assignments map[string]int,
	outputPath string,
	backgroundImagePath ...string,
) error {
	template, err := catalog.GetTemplate(templateID)
	if err != nil {
		return err
	}

	return Render(
		resolveTemplatePath(template.Path),
		photoPaths,
		assignments,
		outputPath,
		backgroundImagePath...,
	)
}

// RenderTemplateAuto fills the template's slots with photos in their given order.
func RenderTemplateAuto(
	catalog TemplateCatalog,
	templateID string,
	photoPaths []string,
	outputPath string,
	backgroundImagePath ...string,
) error {
	template, templatePath, err := loadCatalogTemplate(catalog, templateID)
	if err != nil {
		return err
	}
	if len(photoPaths) != len(template.Slots) {
		return fmt.Errorf("template %s requires %d photos, got %d", templateID, len(template.Slots), len(photoPaths))
	}

	assignments := make(map[string]int, len(template.Slots))
	for index, slot := range template.Slots {
		assignments[slot.ID] = index
	}

	return Render(templatePath, photoPaths, assignments, outputPath, backgroundImagePath...)
}

// RenderTemplateAutoWithText fits the template composition into a reserved
// photo area and places the approved page copy in an editorial footer band.
func RenderTemplateAutoWithText(
	catalog TemplateCatalog,
	templateID string,
	photoPaths []string,
	outputPath string,
	pageText PageText,
	backgroundImagePath ...string,
) error {
	template, templatePath, err := loadCatalogTemplate(catalog, templateID)
	if err != nil {
		return err
	}
	if len(photoPaths) != len(template.Slots) {
		return fmt.Errorf("template %s requires %d photos, got %d", templateID, len(template.Slots), len(photoPaths))
	}
	assignments := make(map[string]int, len(template.Slots))
	for index, slot := range template.Slots {
		assignments[slot.ID] = index
	}
	return RenderWithText(templatePath, photoPaths, assignments, outputPath, pageText, backgroundImagePath...)
}

func TemplatePhotoCount(catalog TemplateCatalog, templateID string) (int, error) {
	template, _, err := loadCatalogTemplate(catalog, templateID)
	if err != nil {
		return 0, err
	}
	return len(template.Slots), nil
}

func loadCatalogTemplate(catalog TemplateCatalog, templateID string) (Template, string, error) {
	templateInfo, err := catalog.GetTemplate(templateID)
	if err != nil {
		return Template{}, "", err
	}
	templatePath := resolveTemplatePath(templateInfo.Path)
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		return Template{}, "", fmt.Errorf("read template: %w", err)
	}
	var template Template
	if err := json.Unmarshal(templateData, &template); err != nil {
		return Template{}, "", fmt.Errorf("parse template: %w", err)
	}
	return template, templatePath, nil
}

func resolveTemplatePath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	parentRelativePath := filepath.Join("..", path)
	if _, err := os.Stat(parentRelativePath); err == nil {
		return parentRelativePath
	}
	return path
}
