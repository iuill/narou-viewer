package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/store"
)

func (s *Server) handleReaderState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		novelID := r.URL.Query().Get("novelId")
		if novelID == "" {
			writeError(w, http.StatusBadRequest, "novelId is required.")
			return
		}
		state, err := s.stateStore.GetReadingState(novelID)
		writeResult(w, state, err)
	case http.MethodPut:
		body, ok := decodeObjectOrBadRequest(w, r)
		if !ok {
			return
		}
		novelID, _ := body["novelId"].(string)
		if novelID == "" {
			writeError(w, http.StatusBadRequest, "novelId is required.")
			return
		}
		lastReadEpisodeIndex, ok := normalizeNullableEpisodeIndex(body, "lastReadEpisodeIndex")
		if !ok {
			writeError(w, http.StatusBadRequest, "lastReadEpisodeIndex must be a non-negative integer string or null.")
			return
		}
		position := 0
		if value, exists := body["position"]; exists && value != nil {
			normalized, ok := store.NormalizePosition(value)
			if !ok {
				writeError(w, http.StatusBadRequest, "position must be a non-negative integer or null.")
				return
			}
			position = normalized
		}
		scroll, ok := store.NormalizeScrollState(body["scroll"])
		if _, exists := body["scroll"]; exists && !ok {
			writeError(w, http.StatusBadRequest, "scroll must be a ratio object.")
			return
		}
		clientID, ok := store.NormalizeClientID(body["clientId"])
		if _, exists := body["clientId"]; exists && !ok {
			writeError(w, http.StatusBadRequest, "clientId must be a non-empty string or null.")
			return
		}
		var expectedStateVersion *int
		value, exists := body["expectedStateVersion"]
		if !exists || value == nil {
			writeError(w, http.StatusBadRequest, "expectedStateVersion must be a non-negative integer.")
			return
		}
		normalized, ok := normalizeInteger(value)
		if !ok || normalized < 0 {
			writeError(w, http.StatusBadRequest, "expectedStateVersion must be a non-negative integer.")
			return
		}
		expectedStateVersion = &normalized
		if s.library != nil {
			exists, err := s.library.NovelExists(novelID)
			if err != nil {
				writeResult(w, nil, err)
				return
			}
			if !exists {
				writeError(w, http.StatusNotFound, "Novel not found.")
				return
			}
		}
		state, err := s.stateStore.PutReadingState(store.ReadingStatePutInput{
			ReadingState: store.ReadingState{
				NovelID:              novelID,
				LastReadEpisodeIndex: lastReadEpisodeIndex,
				Position:             position,
				Scroll:               scroll,
				UpdatedByClientID:    clientID,
			},
			ExpectedStateVersion: expectedStateVersion,
		})
		if errors.Is(err, store.ErrReadingStateVersionConflict) {
			writeReaderStateConflict(w, state)
			return
		}
		writeResult(w, state, err)
	default:
		methodOnly(w, r, http.MethodGet, http.MethodPut)
	}
}

func writeReaderStateConflict(w http.ResponseWriter, state store.ReadingState) {
	writeJSON(w, http.StatusConflict, struct {
		store.ReadingState
		RequestID string `json:"requestId,omitempty"`
	}{
		ReadingState: state,
		RequestID:    strings.TrimSpace(w.Header().Get(apiRequestIDHeader)),
	})
}

func (s *Server) handleReaderPreferences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		preferences, err := s.stateStore.GetReaderPreferences()
		writeResult(w, preferences, err)
	case http.MethodPut:
		body, ok := decodeObjectOrBadRequest(w, r)
		if !ok {
			return
		}
		input := store.ReaderPreferences{}
		if value, exists := body["readingMode"]; exists {
			readingMode, ok := value.(string)
			if !ok || !store.IsReadingMode(readingMode) {
				writeError(w, http.StatusBadRequest, "readingMode must be one of vertical, horizontal.")
				return
			}
			input.ReadingMode = readingMode
		}
		if value, exists := body["fontFamily"]; exists {
			fontFamily, ok := value.(string)
			if !ok || !store.IsReaderFontFamily(fontFamily) {
				writeError(w, http.StatusBadRequest, "fontFamily must be one of mincho, gothic.")
				return
			}
			input.FontFamily = fontFamily
		}
		if value, exists := body["theme"]; exists {
			theme, ok := value.(string)
			if !ok || !store.IsReaderTheme(theme) {
				writeError(w, http.StatusBadRequest, "theme must be one of classic, paper, forest, ocean, midnight.")
				return
			}
			input.Theme = theme
		}
		if input.ReadingMode == "" && input.FontFamily == "" && input.Theme == "" {
			writeError(w, http.StatusBadRequest, "At least one reader preference field is required.")
			return
		}
		preferences, err := s.stateStore.PutReaderPreferences(input)
		writeResult(w, preferences, err)
	default:
		methodOnly(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		bookmarks, err := s.stateStore.ListBookmarks(r.URL.Query().Get("novelId"))
		writeResult(w, map[string][]store.Bookmark{"bookmarks": bookmarks}, err)
	case http.MethodPost:
		body, ok := decodeObjectOrBadRequest(w, r)
		if !ok {
			return
		}
		novelID, _ := body["novelId"].(string)
		episodeIndex, ok := normalizeRequiredEpisodeIndex(body, "episodeIndex")
		if novelID == "" || !ok {
			writeError(w, http.StatusBadRequest, "novelId and episodeIndex are required.")
			return
		}
		position := 0
		if value, exists := body["position"]; exists && value != nil {
			normalized, ok := store.NormalizePosition(value)
			if !ok {
				writeError(w, http.StatusBadRequest, "position must be a non-negative integer.")
				return
			}
			position = normalized
		}
		var label *string
		if rawLabel, ok := body["label"].(string); ok {
			trimmed := strings.TrimSpace(rawLabel)
			if trimmed != "" {
				label = &trimmed
			}
		}
		if s.library != nil {
			exists, err := s.library.NovelExists(novelID)
			if err != nil {
				writeResult(w, nil, err)
				return
			}
			if !exists {
				writeError(w, http.StatusNotFound, "Novel not found.")
				return
			}
		}
		bookmark, err := s.stateStore.CreateBookmark(store.Bookmark{
			NovelID:      novelID,
			EpisodeIndex: episodeIndex,
			Position:     position,
			Label:        label,
		})
		if errors.Is(err, store.ErrNovelStateDeleted) {
			writeError(w, http.StatusGone, "Novel has been deleted.")
			return
		}
		if err != nil {
			if writeStateSchemaError(w, err) {
				return
			}
			writeError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		writeJSON(w, http.StatusCreated, bookmark)
	default:
		methodOnly(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleBookmarkByID(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodDelete) {
		return
	}
	bookmarkID := trimPathValue(strings.TrimPrefix(r.URL.Path, "/api/bookmarks/"))
	if bookmarkID == "" {
		writeError(w, http.StatusNotFound, "Bookmark not found.")
		return
	}
	err := s.stateStore.DeleteBookmark(bookmarkID)
	if errors.Is(err, store.ErrBookmarkNotFound) {
		writeError(w, http.StatusNotFound, "Bookmark not found.")
		return
	}
	writeResult(w, map[string]bool{"deleted": true}, err)
}

func normalizeNullableEpisodeIndex(body map[string]any, key string) (*string, bool) {
	value, exists := body[key]
	if !exists || value == nil {
		return nil, true
	}
	normalized, ok := store.NormalizeEpisodeIndex(value)
	if !ok {
		return nil, false
	}
	return &normalized, true
}

func normalizeInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if math.Trunc(typed) != typed {
			return 0, false
		}
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func normalizeRequiredEpisodeIndex(body map[string]any, key string) (string, bool) {
	value, exists := body[key]
	if !exists || value == nil {
		return "", false
	}
	return store.NormalizeEpisodeIndex(value)
}
