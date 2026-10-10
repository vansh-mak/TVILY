package renderer

import (
	"encoding/json"
	"fmt"
	"os"
)

type TemplateInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type TemplateCatalog struct {
	Templates []TemplateInfo `json:"templates"`
}

func LoadTemplateCatalog(path string) (TemplateCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TemplateCatalog{}, fmt.Errorf("read template catalog: %w", err)
	}

	var catalog TemplateCatalog

	err = json.Unmarshal(data, &catalog)
	if err != nil {
		return TemplateCatalog{}, fmt.Errorf("parse template catalog: %w", err)
	}

	return catalog, nil
}

func (catalog TemplateCatalog) GetTemplate(id string) (TemplateInfo, error) {
	for _, template := range catalog.Templates {
		if template.ID == id {
			return template, nil
		}
	}

	return TemplateInfo{}, fmt.Errorf("template not found: %s", id)
}