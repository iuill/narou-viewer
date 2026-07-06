package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

func (s *Server) handlePublications(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if !s.ensureNovelExists(w, r, novelID) {
		return
	}
	result, err := s.publications.Get(novelID)
	writeResult(w, result, err)
}

func (s *Server) handlePublicationEntries(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	if !s.ensureNovelExists(w, r, novelID) {
		return
	}
	body, ok := decodeJSONOrBadRequest[publications.EntryInput](w, r)
	if !ok {
		return
	}
	result, err := s.publications.CreateEntryWithGoogleBooksAPIKeyResolver(r.Context(), novelID, body, s.googleBooksAPIKeyResolverForPublicationLookup())
	s.writePublicationMutationResult(w, result, err)
}

func (s *Server) handlePublicationEntry(w http.ResponseWriter, r *http.Request, novelID string, entryID string) {
	if !methodOnly(w, r, http.MethodPut) {
		return
	}
	if !s.ensureNovelExists(w, r, novelID) {
		return
	}
	body, ok := decodeJSONOrBadRequest[publications.EntryInput](w, r)
	if !ok {
		return
	}
	result, err := s.publications.PutEntryWithGoogleBooksAPIKeyResolver(r.Context(), novelID, strings.TrimSpace(entryID), body, s.googleBooksAPIKeyResolverForPublicationLookup())
	s.writePublicationMutationResult(w, result, err)
}

func (s *Server) handlePublicationDisplayCover(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodPut) {
		return
	}
	if !s.ensureNovelExists(w, r, novelID) {
		return
	}
	body, ok := decodeJSONOrBadRequest[publications.DisplayCoverInput](w, r)
	if !ok {
		return
	}
	result, err := s.publications.SetDisplayCover(novelID, body)
	s.writePublicationMutationResult(w, result, err)
}

func (s *Server) writePublicationMutationResult(w http.ResponseWriter, result publications.NovelPublications, err error) {
	if errors.Is(err, publications.ErrInvalidKind) {
		writeError(w, http.StatusBadRequest, "kind must be one of novel, comic.")
		return
	}
	if errors.Is(err, publications.ErrInvalidOverride) {
		writeError(w, http.StatusBadRequest, "mode must be one of none, isbn, disabled, visible.")
		return
	}
	if errors.Is(err, publications.ErrInvalidISBN13) {
		writeError(w, http.StatusBadRequest, "ISBN13 が正しくありません。13桁の ISBN を入力してください。")
		return
	}
	if errors.Is(err, publications.ErrInvalidEntry) {
		writeError(w, http.StatusBadRequest, "書籍情報が見つかりません。")
		return
	}
	if store.IsAIGenerationSettingsCryptoError(err) {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeResult(w, result, err)
}

func (s *Server) googleBooksAPIKeyResolverForPublicationLookup() publications.GoogleBooksAPIKeyResolver {
	return s.resolveGoogleBooksAPIKeyForPublicationLookup
}

func (s *Server) resolveGoogleBooksAPIKeyForPublicationLookup() (string, error) {
	if !publications.GoogleBooksEnabled() {
		return "", nil
	}
	if s == nil || s.publications == nil || s.stateStore == nil {
		return "", nil
	}
	apiKey, err := s.stateStore.ResolveGoogleBooksAPIKey()
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

func (s *Server) ensureNovelExists(w http.ResponseWriter, r *http.Request, novelID string) bool {
	if s.library == nil {
		return true
	}
	exists, err := s.library.NovelExists(novelID)
	if err != nil {
		writeResult(w, nil, err)
		return false
	}
	if !exists {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return false
	}
	return true
}
