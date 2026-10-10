package renderer

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/disintegration/imaging"
)

func drawPageText(canvas *image.RGBA, pageText PageText) error {
	bounds := canvas.Bounds()
	bandTop := bounds.Min.Y + int(float64(bounds.Dy())*0.78)
	band := image.Rect(bounds.Min.X, bandTop, bounds.Max.X, bounds.Max.Y)
	draw.Draw(canvas, band, &image.Uniform{C: color.RGBA{R: 20, G: 28, B: 26, A: 242}}, image.Point{}, draw.Over)

	const textScale = 2
	textLayer := image.NewRGBA(image.Rect(0, 0, bounds.Dx()/textScale, band.Dy()/textScale))
	marginX := int(float64(textLayer.Bounds().Dx()) * 0.055)
	maxTextWidth := int(float64(textLayer.Bounds().Dx()) * 0.89)
	baseline := 24
	face := basicfont.Face7x13
	headingLines := wrapToWidth(face, pageText.Heading, maxTextWidth)
	if len(headingLines) > 2 {
		headingLines = headingLines[:2]
		headingLines[1] = ellipsize(face, headingLines[1], maxTextWidth)
	}
	for _, line := range headingLines {
		drawLine(textLayer, face, color.RGBA{R: 244, G: 184, B: 150, A: 255}, marginX, baseline, strings.ToUpper(line))
		baseline += 18
	}
	if len(headingLines) > 0 {
		baseline += 7
	} else {
		baseline += 4
	}
	bodyLines := wrapToWidth(face, strings.TrimSpace(pageText.Text), maxTextWidth)
	maxBodyLines := 5
	if len(bodyLines) > maxBodyLines {
		bodyLines = bodyLines[:maxBodyLines]
		bodyLines[maxBodyLines-1] = ellipsize(face, bodyLines[maxBodyLines-1], maxTextWidth)
	}
	lineHeight := 17
	for _, line := range bodyLines {
		if baseline > textLayer.Bounds().Max.Y-5 {
			break
		}
		drawLine(textLayer, face, color.RGBA{R: 248, G: 246, B: 238, A: 255}, marginX, baseline, line)
		baseline += lineHeight
	}
	scaledText := imaging.Resize(textLayer, band.Dx(), band.Dy(), imaging.NearestNeighbor)
	draw.Draw(canvas, band, scaledText, image.Point{}, draw.Over)
	return nil
}

func drawLine(dst *image.RGBA, face font.Face, color color.Color, x, baseline int, text string) {
	drawer := font.Drawer{Dst: dst, Src: image.NewUniform(color), Face: face, Dot: fixed.P(x, baseline)}
	drawer.DrawString(text)
}

func wrapToWidth(face font.Face, text string, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(strings.TrimSpace(text), "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			candidate := line + " " + word
			if font.MeasureString(face, candidate).Ceil() <= width {
				line = candidate
				continue
			}
			lines = append(lines, line)
			line = word
		}
		lines = append(lines, line)
	}
	return lines
}

func ellipsize(face font.Face, text string, width int) string {
	runes := []rune(strings.TrimSpace(text))
	for len(runes) > 0 {
		candidate := string(runes) + "…"
		if font.MeasureString(face, candidate).Ceil() <= width {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
}
