package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
)

const pageWidth = 432
const pageHeight = 540

func CreateFromPNGs(imagePaths []string, outputPath string) error {
	if len(imagePaths) == 0 {
		return fmt.Errorf("cannot create a PDF without pages")
	}

	objects := make([][]byte, 2+3*len(imagePaths))
	objects[0] = []byte("<< /Type /Catalog /Pages 2 0 R >>")

	var pageRefs bytes.Buffer
	fmt.Fprintf(&pageRefs, "<< /Type /Pages /Count %d /Kids [", len(imagePaths))
	for index := range imagePaths {
		pageObjectID := 3 + index*3
		fmt.Fprintf(&pageRefs, "%d 0 R ", pageObjectID)
	}
	pageRefs.WriteString("] >>")
	objects[1] = pageRefs.Bytes()

	for index, path := range imagePaths {
		pageObjectID := 3 + index*3
		contentObjectID := pageObjectID + 1
		imageObjectID := pageObjectID + 2

		imageFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open page image %s: %w", path, err)
		}
		pageImage, err := png.Decode(imageFile)
		closeErr := imageFile.Close()
		if err != nil {
			return fmt.Errorf("decode page image %s: %w", path, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close page image %s: %w", path, closeErr)
		}

		compressedPixels, width, height, err := compressRGB(pageImage)
		if err != nil {
			return fmt.Errorf("prepare page image %s: %w", path, err)
		}

		objects[pageObjectID-1] = []byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
			pageWidth, pageHeight, imageObjectID, contentObjectID,
		))
		content := []byte(fmt.Sprintf("q %d 0 0 %d 0 0 cm /Im0 Do Q", pageWidth, pageHeight))
		objects[contentObjectID-1] = streamObject(content, fmt.Sprintf("<< /Length %d >>", len(content)))
		objects[imageObjectID-1] = streamObject(compressedPixels, fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>",
			width, height, len(compressedPixels),
		))
	}

	return writePDF(objects, outputPath)
}

func compressRGB(source image.Image) ([]byte, int, int, error) {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, 0, 0, fmt.Errorf("image has invalid dimensions")
	}

	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	row := make([]byte, width*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			index := (x - bounds.Min.X) * 3
			row[index] = pixel.R
			row[index+1] = pixel.G
			row[index+2] = pixel.B
		}
		if _, err := writer.Write(row); err != nil {
			_ = writer.Close()
			return nil, 0, 0, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, 0, 0, err
	}
	return compressed.Bytes(), width, height, nil
}

func streamObject(data []byte, dictionary string) []byte {
	var object bytes.Buffer
	object.WriteString(dictionary)
	object.WriteString("\nstream\n")
	object.Write(data)
	object.WriteString("\nendstream")
	return object.Bytes()
}

func writePDF(objects [][]byte, outputPath string) error {
	var document bytes.Buffer
	document.WriteString("%PDF-1.4\n%TVILY\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = document.Len()
		fmt.Fprintf(&document, "%d 0 obj\n", index+1)
		document.Write(object)
		document.WriteString("\nendobj\n")
	}
	xrefOffset := document.Len()
	fmt.Fprintf(&document, "xref\n0 %d\n", len(objects)+1)
	document.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&document, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&document, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create PDF: %w", err)
	}
	if _, err := io.Copy(file, &document); err != nil {
		_ = file.Close()
		return fmt.Errorf("write PDF: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close PDF: %w", err)
	}
	return nil
}
