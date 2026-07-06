package readerview

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

var ErrNovelNotFound = errors.New("novel not found")

type LibraryPort interface {
	GetEpisode(context.Context, string, string) (*library.EpisodeResponse, error)
	NovelExists(string) (bool, error)
}

type StatePort interface {
	GetNovelReaderSettings(string) (store.NovelReaderSettings, error)
	PatchNovelReaderSettings(string, store.NovelReaderCorrectionPatch) (store.NovelReaderSettings, error)
}

type EpisodeView struct {
	Episode *library.EpisodeResponse
	ETag    string
}

type Service struct {
	library   LibraryPort
	state     StatePort
	textCache *readertextcache.Store
}

func NewService(library LibraryPort, state StatePort) *Service {
	return NewServiceWithTextCache(library, state, nil)
}

func NewServiceWithTextCache(library LibraryPort, state StatePort, textCache *readertextcache.Store) *Service {
	return &Service{library: library, state: state, textCache: textCache}
}

func (s *Service) GetEpisode(ctx context.Context, novelID string, episodeIndex string) (EpisodeView, error) {
	if s == nil || s.library == nil {
		return EpisodeView{}, nil
	}
	episode, err := s.library.GetEpisode(ctx, novelID, episodeIndex)
	if err != nil || episode == nil {
		return EpisodeView{Episode: episode}, err
	}
	var readerSettings store.NovelReaderSettings
	if s.state != nil {
		readerSettings, err = s.state.GetNovelReaderSettings(novelID)
		if err != nil {
			return EpisodeView{}, err
		}
	}
	if s.textCache != nil && readerSearchCacheableETag(episode.ContentEtag) {
		_ = s.textCache.Save(ctx, novelID, episodeIndex, episode.ContentEtag, readertextcache.BodyText(episode.ReaderDocument))
	}
	correctionSettings := readerCorrectionSettings(readerSettings)
	responseETag := EpisodeResponseETag(episode.ContentEtag, correctionSettings)
	episode.ReaderDocument = library.ApplyReaderCorrections(episode.ReaderDocument, correctionSettings)
	episode.ContentEtag = responseETag
	return EpisodeView{Episode: episode, ETag: responseETag}, nil
}

func (s *Service) GetSettings(novelID string) (store.NovelReaderSettings, error) {
	if s == nil || s.state == nil {
		return store.NovelReaderSettings{}, nil
	}
	return s.state.GetNovelReaderSettings(novelID)
}

func (s *Service) PatchSettings(novelID string, patch store.NovelReaderCorrectionPatch) (store.NovelReaderSettings, error) {
	if s != nil && s.library != nil {
		exists, err := s.library.NovelExists(novelID)
		if err != nil {
			return store.NovelReaderSettings{}, err
		}
		if !exists {
			return store.NovelReaderSettings{}, ErrNovelNotFound
		}
	}
	if s == nil || s.state == nil {
		return store.NovelReaderSettings{}, nil
	}
	return s.state.PatchNovelReaderSettings(novelID, patch)
}

func readerSearchCacheableETag(contentETag string) bool {
	contentETag = strings.TrimSpace(contentETag)
	if contentETag == "" {
		return false
	}
	if len(contentETag) != 64 {
		return true
	}
	for _, r := range contentETag {
		if !unicode.IsDigit(r) && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return true
		}
	}
	return false
}

func EpisodeResponseETag(contentETag string, correctionSettings library.ReaderCorrectionSettings) string {
	quoteState := "q0"
	if correctionSettings.QuoteNormalization {
		quoteState = "q1"
	}
	hyphenState := "h0"
	if correctionSettings.HyphenDashNormalization {
		hyphenState = "h1"
	}
	parenthesisState := "p0"
	if correctionSettings.ParenthesisNormalization {
		parenthesisState = "p1"
	}
	halfwidthState := "a0"
	if correctionSettings.HalfwidthAlnumPunctuationNormalization {
		halfwidthState = "a1"
	}
	return contentETag + "-reader-corrections-" + quoteState + hyphenState + parenthesisState + halfwidthState
}

func readerCorrectionSettings(settings store.NovelReaderSettings) library.ReaderCorrectionSettings {
	return library.ReaderCorrectionSettings{
		QuoteNormalization:                     settings.Correction.QuoteNormalization,
		HyphenDashNormalization:                settings.Correction.HyphenDashNormalization,
		ParenthesisNormalization:               settings.Correction.ParenthesisNormalization,
		HalfwidthAlnumPunctuationNormalization: settings.Correction.HalfwidthAlnumPunctuationNormalization,
	}
}
