package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/store"
)

const maxLibraryImportJSONBodyBytes int64 = 2 << 20

type libraryImportRequest struct {
	DryRun   bool                  `json:"dryRun"`
	Document libraryExportDocument `json:"document"`
}

type libraryExportDocument struct {
	FormatVersion  int                    `json:"formatVersion"`
	ExportedAt     string                 `json:"exportedAt"`
	NovelsCount    int                    `json:"novelsCount"`
	ExportWarnings []libraryExportWarning `json:"exportWarnings"`
	Novels         []libraryExportNovel   `json:"novels"`
}

type libraryExportWarning struct {
	NovelID string `json:"novelId"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type libraryExportNovel struct {
	NovelID       string                     `json:"novelId"`
	FetcherWorkID string                     `json:"fetcherWorkId"`
	Title         string                     `json:"title"`
	Author        string                     `json:"author"`
	SiteName      string                     `json:"siteName"`
	TocURL        *string                    `json:"tocUrl"`
	UpdatedAt     *string                    `json:"updatedAt"`
	LastActivity  *string                    `json:"lastActivityAt"`
	TotalEpisodes int                        `json:"totalEpisodes"`
	SavedEpisodes *int                       `json:"savedEpisodes"`
	FetchStatus   *string                    `json:"fetchStatus"`
	ReadingState  *libraryExportReadingState `json:"readingState"`
	Bookmarks     []libraryExportBookmark    `json:"bookmarks"`
}

type libraryExportReadingState struct {
	LastReadEpisodeIndex *string `json:"lastReadEpisodeIndex"`
	Position             int     `json:"position"`
	UpdatedAt            *string `json:"updatedAt"`
}

type libraryExportBookmark struct {
	ID           string  `json:"id"`
	NovelID      string  `json:"novelId"`
	EpisodeIndex string  `json:"episodeIndex"`
	Position     int     `json:"position"`
	Label        *string `json:"label"`
	CreatedAt    string  `json:"createdAt"`
}

type libraryImportResponse struct {
	DryRun        bool `json:"dryRun"`
	NovelsMatched int  `json:"novelsMatched"`
	NovelsSkipped int  `json:"novelsSkipped"`
	store.LibraryImportResult
	Warnings []string `json:"warnings"`
}

func (s *Server) handleLibraryImport(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	request, ok := decodeStrictLibraryImportRequest(w, r)
	if !ok {
		return
	}
	if err := validateLibraryExportDocument(request.Document); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	response := libraryImportResponse{DryRun: request.DryRun, Warnings: []string{}}
	importNovels := make([]store.LibraryImportNovel, 0, len(request.Document.Novels))
	for _, novel := range request.Document.Novels {
		exists, err := s.library.NovelExists(novel.NovelID)
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		if !exists {
			response.NovelsSkipped++
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s (%s): 未取得作品のためスキップしました。", novel.Title, novel.NovelID))
			continue
		}

		importNovel := store.LibraryImportNovel{NovelID: novel.NovelID, Bookmarks: []store.Bookmark{}}
		if novel.ReadingState != nil && novel.ReadingState.LastReadEpisodeIndex != nil {
			_, episodeExists, err := s.library.EpisodeExists(r.Context(), novel.NovelID, *novel.ReadingState.LastReadEpisodeIndex)
			if err != nil {
				writeResult(w, nil, err)
				return
			}
			if episodeExists {
				importNovel.ReadingState = &store.ReadingState{
					NovelID:              novel.NovelID,
					LastReadEpisodeIndex: novel.ReadingState.LastReadEpisodeIndex,
					Position:             novel.ReadingState.Position,
				}
			} else {
				response.Warnings = append(response.Warnings, fmt.Sprintf("%s (%s): 既読話が存在しないため既読位置をスキップしました。", novel.Title, novel.NovelID))
			}
		}
		for _, bookmark := range novel.Bookmarks {
			_, episodeExists, err := s.library.EpisodeExists(r.Context(), novel.NovelID, bookmark.EpisodeIndex)
			if err != nil {
				writeResult(w, nil, err)
				return
			}
			if !episodeExists {
				response.Warnings = append(response.Warnings, fmt.Sprintf("%s (%s): 存在しない話の栞をスキップしました。", novel.Title, novel.NovelID))
				continue
			}
			importNovel.Bookmarks = append(importNovel.Bookmarks, store.Bookmark{
				NovelID:      novel.NovelID,
				EpisodeIndex: bookmark.EpisodeIndex,
				Position:     bookmark.Position,
				Label:        bookmark.Label,
			})
		}
		response.NovelsMatched++
		importNovels = append(importNovels, importNovel)
	}

	result, err := s.stateStore.ImportLibrary(importNovels, request.DryRun)
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	response.LibraryImportResult = result
	writeJSON(w, http.StatusOK, response)
}

func decodeStrictLibraryImportRequest(w http.ResponseWriter, r *http.Request) (libraryImportRequest, bool) {
	if !hasJSONContentType(r) {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json.")
		return libraryImportRequest{}, false
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLibraryImportJSONBodyBytes))
	decoder.DisallowUnknownFields()
	var request libraryImportRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid library export document.")
		return libraryImportRequest{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Invalid library export document.")
		return libraryImportRequest{}, false
	}
	return request, true
}

func validateLibraryExportDocument(document libraryExportDocument) error {
	if document.FormatVersion != 1 {
		return fmt.Errorf("Unsupported library export formatVersion.")
	}
	if _, err := time.Parse(time.RFC3339, document.ExportedAt); err != nil {
		return fmt.Errorf("exportedAt must be an RFC3339 timestamp.")
	}
	if document.NovelsCount != len(document.Novels) {
		return fmt.Errorf("novelsCount does not match novels.")
	}
	seenNovelIDs := map[string]struct{}{}
	for _, warning := range document.ExportWarnings {
		if strings.TrimSpace(warning.NovelID) == "" || warning.Field != "readingState" || strings.TrimSpace(warning.Message) == "" {
			return fmt.Errorf("exportWarnings contains an invalid entry.")
		}
	}
	for _, novel := range document.Novels {
		if strings.TrimSpace(novel.NovelID) == "" || strings.TrimSpace(novel.FetcherWorkID) == "" ||
			strings.TrimSpace(novel.Title) == "" || strings.TrimSpace(novel.Author) == "" ||
			strings.TrimSpace(novel.SiteName) == "" || novel.TotalEpisodes < 0 {
			return fmt.Errorf("novels contains an invalid entry.")
		}
		if _, exists := seenNovelIDs[novel.NovelID]; exists {
			return fmt.Errorf("novels contains duplicate novelId.")
		}
		seenNovelIDs[novel.NovelID] = struct{}{}
		if novel.ReadingState != nil {
			if novel.ReadingState.Position < 0 ||
				(novel.ReadingState.LastReadEpisodeIndex != nil && !store.IsEpisodeIndex(*novel.ReadingState.LastReadEpisodeIndex)) {
				return fmt.Errorf("readingState contains an invalid entry.")
			}
		}
		for _, bookmark := range novel.Bookmarks {
			if strings.TrimSpace(bookmark.ID) == "" || bookmark.NovelID != novel.NovelID ||
				!store.IsEpisodeIndex(bookmark.EpisodeIndex) || bookmark.Position < 0 ||
				strings.TrimSpace(bookmark.CreatedAt) == "" {
				return fmt.Errorf("bookmarks contains an invalid entry.")
			}
			if _, err := time.Parse(time.RFC3339, bookmark.CreatedAt); err != nil {
				return fmt.Errorf("bookmark createdAt must be an RFC3339 timestamp.")
			}
		}
	}
	return nil
}
