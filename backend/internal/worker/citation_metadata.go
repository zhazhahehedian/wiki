package worker

import (
	"encoding/json"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type citationSource struct {
	SourceType     string                  `json:"source_type"`
	SourceURL      string                  `json:"source_url"`
	SectionPath    string                  `json:"section_path"`
	SheetName      string                  `json:"sheet_name"`
	SheetID        string                  `json:"sheet_id"`
	TableID        string                  `json:"table_id"`
	ViewID         string                  `json:"view_id"`
	RowStart       int                     `json:"row_start"`
	RowEnd         int                     `json:"row_end"`
	RemoteRevision string                  `json:"remote_revision"`
	Locations      []domain.SourceLocation `json:"locations"`
}

func remoteCitationMetadata(raw json.RawMessage, sectionPath string) map[string]any {
	var source citationSource
	_ = json.Unmarshal(raw, &source)
	metadata := map[string]any{}
	putText(metadata, "source_type", source.SourceType)
	putText(metadata, "source_url", source.SourceURL)
	putText(metadata, "remote_revision", source.RemoteRevision)
	if sectionPath == "" {
		sectionPath = source.SectionPath
	}
	putText(metadata, "section_path", sectionPath)

	location := domain.SourceLocation{
		SectionPath: source.SectionPath, SheetName: source.SheetName, SheetID: source.SheetID,
		TableID: source.TableID, ViewID: source.ViewID, RowStart: source.RowStart, RowEnd: source.RowEnd,
	}
	for _, candidate := range source.Locations {
		if candidate.SectionPath == sectionPath {
			location = candidate
			break
		}
	}
	putText(metadata, "sheet_name", location.SheetName)
	putText(metadata, "sheet_id", location.SheetID)
	putText(metadata, "table_id", location.TableID)
	putText(metadata, "view_id", location.ViewID)
	if location.RowStart > 0 && location.RowEnd >= location.RowStart {
		metadata["row_start"] = location.RowStart
		metadata["row_end"] = location.RowEnd
	}
	return metadata
}

func putText(metadata map[string]any, key, value string) {
	if value != "" {
		metadata[key] = value
	}
}
