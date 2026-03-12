package tools

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type ViewParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the file to read"`
	Offset   int    `json:"offset,omitempty" jsonschema:"description=Line number to start from (1-based)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Number of lines to read (default 2000)"`
}

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
	maxImageSize     = 20 * 1024 * 1024 // 20 MB
)

// imageExtensions maps file extensions to MIME types for supported image formats.
var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

func ViewTool(workingDir string) core.Tool {
	return core.FuncTool[ViewParams](
		"view",
		"Read a file's contents. Returns the file with line numbers. "+
			"Use offset and limit for large files. Supports text files and images (png, jpg, gif, webp, svg).",
		func(ctx context.Context, params ViewParams) (any, error) {
			if params.FilePath == "" {
				return nil, errors.New("file_path is required")
			}

			path := resolvePath(workingDir, params.FilePath)

			// Check for image files.
			ext := strings.ToLower(filepath.Ext(path))
			if mime, ok := imageExtensions[ext]; ok {
				return readImage(path, mime)
			}

			// Text file reading.
			limit := params.Limit
			if limit <= 0 {
				limit = defaultReadLimit
			}

			f, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("opening file: %w", err)
			}
			defer f.Close()

			var lines []string
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				if params.Offset > 0 && lineNum < params.Offset {
					continue
				}
				if len(lines) >= limit {
					break
				}
				line := scanner.Text()
				if len(line) > maxLineLength {
					line = line[:maxLineLength] + "..."
				}
				lines = append(lines, fmt.Sprintf("%4d│ %s", lineNum, line))
			}

			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("reading file: %w", err)
			}

			if len(lines) == 0 {
				return "(empty file)", nil
			}

			return strings.Join(lines, "\n"), nil
		},
	)
}

// readImage reads an image file and returns a ToolResultWithImages containing
// the base64-encoded image data for multimodal LLM consumption.
func readImage(path, mimeType string) (any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat image: %w", err)
	}
	if info.Size() > maxImageSize {
		return nil, fmt.Errorf("image too large: %d bytes (max %d)", info.Size(), maxImageSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading image: %w", err)
	}

	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
	return core.ToolResultWithImages{
		Text: fmt.Sprintf("[Image: %s (%s, %d bytes)]", filepath.Base(path), mimeType, len(data)),
		Images: []core.ImagePart{
			{URL: dataURI, MIMEType: mimeType},
		},
	}, nil
}

func resolvePath(workingDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workingDir, path)
}
