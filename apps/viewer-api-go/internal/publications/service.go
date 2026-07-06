package publications

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidKind = errors.New("invalid publication kind")
var ErrInvalidOverride = errors.New("invalid publication override")
var ErrInvalidISBN13 = errors.New("invalid isbn13")
var ErrInvalidEntry = errors.New("invalid publication entry")

type Service struct {
	repository *Repository
	ndl        *NDLClient
	google     *GoogleBooksClient
	now        func() time.Time
}

type GoogleBooksAPIKeyResolver func() (string, error)

func NewService(stateDir string) *Service {
	return &Service{
		repository: NewRepository(stateDir),
		ndl:        NewNDLClientFromEnv(),
		google:     NewGoogleBooksClientFromEnv(),
		now:        time.Now,
	}
}

func (s *Service) Ensure() error {
	if s == nil || s.repository == nil {
		return nil
	}
	return s.repository.Ensure()
}

func (s *Service) Get(novelID string) (NovelPublications, error) {
	if s == nil || s.repository == nil {
		return normalizeNovelPublications(NovelPublications{}, novelID), nil
	}
	return s.repository.Get(novelID)
}

func (s *Service) ListByNovelIDs(novelIDs []string) (map[string]NovelPublications, error) {
	result := make(map[string]NovelPublications, len(novelIDs))
	if s == nil || s.repository == nil {
		for _, novelID := range novelIDs {
			novelID = strings.TrimSpace(novelID)
			if novelID != "" {
				result[novelID] = normalizeNovelPublications(NovelPublications{}, novelID)
			}
		}
		return result, nil
	}
	return s.repository.ListByNovelIDs(novelIDs)
}

func (s *Service) CreateEntry(ctx context.Context, novelID string, input EntryInput) (NovelPublications, error) {
	return s.CreateEntryWithGoogleBooksAPIKey(ctx, novelID, input, "")
}

func (s *Service) CreateEntryWithGoogleBooksAPIKey(ctx context.Context, novelID string, input EntryInput, googleBooksAPIKey string) (NovelPublications, error) {
	return s.CreateEntryWithGoogleBooksAPIKeyResolver(ctx, novelID, input, staticGoogleBooksAPIKeyResolver(googleBooksAPIKey))
}

func (s *Service) CreateEntryWithGoogleBooksAPIKeyResolver(ctx context.Context, novelID string, input EntryInput, googleBooksAPIKeyResolver GoogleBooksAPIKeyResolver) (NovelPublications, error) {
	kind := normalizeKind(input.Kind)
	if kind == "" {
		return NovelPublications{}, ErrInvalidKind
	}
	if input.Mode != OverrideModeISBN {
		return NovelPublications{}, ErrInvalidOverride
	}
	isbn13 := NormalizeISBN13(input.ISBN13)
	if isbn13 == "" {
		return NovelPublications{}, ErrInvalidISBN13
	}
	current, err := s.Get(novelID)
	if err != nil {
		return NovelPublications{}, err
	}
	for _, entry := range current.Entries {
		if entry.Kind == kind && entry.ISBN13 == isbn13 {
			return s.putISBNEntry(ctx, novelID, entry.ID, kind, isbn13, googleBooksAPIKeyResolver)
		}
	}
	return s.putISBNEntry(ctx, novelID, s.nextEntryID(novelID, kind, isbn13), kind, isbn13, googleBooksAPIKeyResolver)
}

func (s *Service) PutEntry(ctx context.Context, novelID string, entryID string, input EntryInput) (NovelPublications, error) {
	return s.PutEntryWithGoogleBooksAPIKey(ctx, novelID, entryID, input, "")
}

func (s *Service) PutEntryWithGoogleBooksAPIKey(ctx context.Context, novelID string, entryID string, input EntryInput, googleBooksAPIKey string) (NovelPublications, error) {
	return s.PutEntryWithGoogleBooksAPIKeyResolver(ctx, novelID, entryID, input, staticGoogleBooksAPIKeyResolver(googleBooksAPIKey))
}

func (s *Service) PutEntryWithGoogleBooksAPIKeyResolver(ctx context.Context, novelID string, entryID string, input EntryInput, googleBooksAPIKeyResolver GoogleBooksAPIKeyResolver) (NovelPublications, error) {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return NovelPublications{}, ErrInvalidEntry
	}
	current, err := s.Get(novelID)
	if err != nil {
		return NovelPublications{}, err
	}
	entry, ok := findEntry(current.Entries, entryID)
	if !ok {
		return NovelPublications{}, ErrInvalidEntry
	}
	kind := normalizeKind(input.Kind)
	if input.Kind != "" && kind == "" {
		return NovelPublications{}, ErrInvalidKind
	}
	if kind != "" && kind != entry.Kind {
		return NovelPublications{}, ErrInvalidEntry
	}
	if kind == "" {
		kind = entry.Kind
	}
	if kind == "" {
		return NovelPublications{}, ErrInvalidKind
	}
	now := s.now().UTC().Format(time.RFC3339)
	switch input.Mode {
	case OverrideModeNone, "":
		return s.repository.DeleteEntry(novelID, entryID)
	case OverrideModeDisabled:
		entry.Kind = kind
		entry.Status = EntryStatusDisabled
		entry.Override = OverrideModeDisabled
		entry.UpdatedAt = now
		return s.repository.PutEntry(novelID, entry)
	case OverrideModeVisible:
		entry.Kind = kind
		if entry.ISBN13 != "" {
			entry.Status = EntryStatusManual
			entry.Override = OverrideModeISBN
		} else {
			entry.Status = EntryStatusUnknown
			entry.Override = OverrideModeNone
		}
		entry.UpdatedAt = now
		return s.repository.PutEntry(novelID, entry)
	case OverrideModeISBN:
		isbn13 := NormalizeISBN13(input.ISBN13)
		if isbn13 == "" {
			return NovelPublications{}, ErrInvalidISBN13
		}
		for _, currentEntry := range current.Entries {
			if currentEntry.ID != entryID && currentEntry.Kind == kind && currentEntry.ISBN13 == isbn13 {
				return s.putISBNEntryDeleting(ctx, novelID, currentEntry.ID, entryID, kind, isbn13, googleBooksAPIKeyResolver)
			}
		}
		return s.putISBNEntry(ctx, novelID, entryID, kind, isbn13, googleBooksAPIKeyResolver)
	default:
		return NovelPublications{}, ErrInvalidOverride
	}
}

func (s *Service) SetDisplayCover(novelID string, input DisplayCoverInput) (NovelPublications, error) {
	entryID := strings.TrimSpace(input.EntryID)
	if entryID == "" {
		return s.repository.PutDisplayCoverEntryID(novelID, "")
	}
	current, err := s.Get(novelID)
	if err != nil {
		return NovelPublications{}, err
	}
	for _, entry := range current.Entries {
		if entry.ID == entryID && entry.Status != EntryStatusDisabled && entry.ImageURL != "" {
			return s.repository.PutDisplayCoverEntryID(novelID, entryID)
		}
	}
	return NovelPublications{}, ErrInvalidEntry
}

func (s *Service) putISBNEntry(ctx context.Context, novelID string, entryID string, kind Kind, isbn13 string, googleBooksAPIKeyResolver GoogleBooksAPIKeyResolver) (NovelPublications, error) {
	return s.putISBNEntryDeleting(ctx, novelID, entryID, "", kind, isbn13, googleBooksAPIKeyResolver)
}

func (s *Service) putISBNEntryDeleting(ctx context.Context, novelID string, entryID string, deleteEntryID string, kind Kind, isbn13 string, googleBooksAPIKeyResolver GoogleBooksAPIKeyResolver) (NovelPublications, error) {
	now := s.now().UTC().Format(time.RFC3339)
	entry := Entry{
		ID:        entryID,
		Kind:      kind,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    isbn13,
		UpdatedAt: now,
		CheckedAt: now,
	}
	if NDLSearchEnabled() && s.ndl != nil {
		bibliography, err := s.ndl.LookupISBN(ctx, isbn13)
		if err != nil && ctx.Err() != nil {
			return NovelPublications{}, err
		}
		if err == nil {
			entry = mergeNDLBibliography(entry, bibliography)
		} else {
			entry.Warnings = append(entry.Warnings, "ndl_lookup_failed")
		}
	}
	if GoogleBooksEnabled() && s.google != nil {
		googleBooksAPIKey, err := resolveGoogleBooksAPIKey(googleBooksAPIKeyResolver)
		if err != nil {
			return NovelPublications{}, err
		}
		if googleBooksAPIKey == "" {
			googleBooksAPIKey = s.google.APIKey()
		}
		if googleBooksAPIKey == "" {
			entry.Warnings = append(entry.Warnings, "google_books_api_key_missing")
		} else {
			volume, err := s.google.LookupISBNWithAPIKey(ctx, isbn13, googleBooksAPIKey)
			if err != nil && ctx.Err() != nil {
				return NovelPublications{}, err
			}
			if err == nil {
				entry = mergeGoogleBooksVolume(entry, volume)
				if volume != nil && volume.ImageURL == "" {
					entry.Warnings = append(entry.Warnings, "google_books_cover_missing")
				}
			} else {
				entry.Warnings = append(entry.Warnings, "google_books_lookup_failed")
			}
		}
	}
	if strings.TrimSpace(deleteEntryID) != "" {
		return s.repository.PutEntryDeleting(novelID, entry, deleteEntryID)
	}
	return s.repository.PutEntry(novelID, entry)
}

func staticGoogleBooksAPIKeyResolver(apiKey string) GoogleBooksAPIKeyResolver {
	return func() (string, error) {
		return apiKey, nil
	}
}

func resolveGoogleBooksAPIKey(resolver GoogleBooksAPIKeyResolver) (string, error) {
	if resolver == nil {
		return "", nil
	}
	apiKey, err := resolver()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(apiKey), nil
}

func (s *Service) nextEntryID(novelID string, kind Kind, isbn13 string) string {
	base := string(kind) + "-" + isbn13
	current, err := s.Get(novelID)
	if err != nil {
		return base
	}
	seen := map[string]bool{}
	for _, entry := range current.Entries {
		seen[entry.ID] = true
	}
	if !seen[base] {
		return base
	}
	return base + "-" + s.now().UTC().Format("20060102150405.000000000")
}

func findEntry(entries []Entry, entryID string) (Entry, bool) {
	for _, entry := range entries {
		if entry.ID == entryID {
			return entry, true
		}
	}
	return Entry{}, false
}

func mergeNDLBibliography(entry Entry, bibliography *NDLBibliography) Entry {
	if bibliography == nil {
		return entry
	}
	entry.Title = bibliography.Title
	entry.Authors = bibliography.Authors
	entry.Publisher = bibliography.Publisher
	entry.Published = bibliography.PublishedDate
	entry.DetailURL = bibliography.DetailURL
	entry.ProviderID = appendProviderID(entry.ProviderID, ndlProviderID)
	entry.Source = "NDLサーチ"
	entry.SourceURL = bibliography.DetailURL
	return entry
}

func mergeGoogleBooksVolume(entry Entry, volume *GoogleBooksVolume) Entry {
	if volume == nil {
		return entry
	}
	if entry.Title == "" {
		entry.Title = volume.Title
	}
	if entry.Subtitle == "" {
		entry.Subtitle = volume.Subtitle
	}
	if len(entry.Authors) == 0 {
		entry.Authors = volume.Authors
	}
	if entry.Publisher == "" {
		entry.Publisher = volume.Publisher
	}
	if entry.Published == "" {
		entry.Published = volume.PublishedDate
	}
	if entry.DetailURL == "" {
		entry.DetailURL = firstNonEmpty(volume.CanonicalVolumeLink, volume.InfoLink)
	}
	entry.ImageURL = volume.ImageURL
	entry.ProviderID = appendProviderID(entry.ProviderID, googleBooksProviderID)
	entry.CoverSource = "Google Books"
	entry.CoverSourceURL = firstNonEmpty(volume.CanonicalVolumeLink, volume.InfoLink)
	if entry.Source == "" {
		entry.Source = "Google Books"
		entry.SourceURL = entry.CoverSourceURL
	}
	return entry
}

func appendProviderID(current string, providerID string) string {
	current = strings.TrimSpace(current)
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return current
	}
	if current == "" {
		return providerID
	}
	for _, existing := range strings.Split(current, "+") {
		if existing == providerID {
			return current
		}
	}
	return current + "+" + providerID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
