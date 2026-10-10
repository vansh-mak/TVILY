# TVILY

TVILY turns a set of travel photos into a multi-page photo magazine PDF. It selects a compatible composition from the local template catalog, generates a themed background for the pages, places the uploaded photos, and assembles the pages into a downloadable PDF.

## Current capabilities

- Accepts 2–20 JPEG, PNG, or WebP photos per upload, up to 50 MB total.
- Creates one page per selected layout, or chooses layouts locally based on the requested page and photo counts.
- Supports repeated layouts when using the generation API directly.
- Uses the OpenRouter **image model** for page backgrounds. Photo analysis, AI layout planning, captions, and prose are currently disabled in the active web flow.
- Renders a PDF preview that can be downloaded from the browser.

Trip context currently guides the generated page background. It does not drive photo analysis or layout selection.

## Requirements

- Go 1.26.5 or later
- An OpenRouter API key with access to an image generation model and sufficient credits

## Configuration

Create a `.env` file in the project root:

```env
OPENROUTER_API_KEY=your_openrouter_api_key
OPENROUTER_IMAGE_MODEL=black-forest-labs/flux.2-pro
```

Keep the API key private. `.env` and generated job files under `temp/` are ignored by Git.

The configured model must support image generation and the request parameters TVILY sends: 1K resolution, 3:4 aspect ratio, PNG output, and one image. Model availability, provider support, and cost can vary.

## Run locally

From the project root:

```powershell
go run .
```

Open <http://localhost:8080>. The health endpoint is <http://localhost:8080/health>.

## Generation flow

1. The browser sends photos to `POST /upload`. The response includes a `job_id`.
2. The browser sends trip context and optional page settings to `POST /generate/{job_id}`. TVILY saves a local plan in `temp/{job_id}/prepared_magazine.json`.
3. The browser calls `POST /render/{job_id}`. TVILY generates or reuses page backgrounds, renders each page to PNG, and returns a PDF.

The generation endpoint accepts these query parameters:

| Parameter | Purpose |
| --- | --- |
| `trip_context` | Required context used as background art direction |
| `page_count` | Optional number of pages, from 1 to 20 |
| `photos_per_page` | Optional target of 2–6 photos per page |
| `layout` | Optional repeated parameter to specify a template ID per page; the selected layouts must account for every uploaded photo |

Each generated page uses a compatible template from `templates/templates.json`. Template definitions are grouped into family folders under `templates/` and specify canvas dimensions and photo slots.

## Project structure

| Path | Responsibility |
| --- | --- |
| `main.go` | Loads configuration and registers HTTP routes |
| `handlers/` | Upload, plan, and render request handling |
| `ai/background.go` | OpenRouter image API request and background-image saving |
| `ai/` | AI request and data-validation code; some text-model helpers are currently inactive |
| `renderer/` | Loads templates and renders photos and backgrounds into page images |
| `templates/` | Template catalog and JSON layout definitions, grouped by family |
| `pdf/pdf.go` | Converts rendered page images into a PDF |
| `web/index.html` | Browser upload, progress, PDF preview, and download interface |
| `temp/` | Per-job uploads and generated intermediate/output files; ignored by Git |

