package publications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const googleBooksProviderID = "google_books"

type GoogleBooksClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewGoogleBooksClientFromEnv() *GoogleBooksClient {
	baseURL := strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://www.googleapis.com/books/v1"
	}
	return &GoogleBooksClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_KEY")),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func GoogleBooksEnabled() bool {
	value := strings.TrimSpace(os.Getenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED"))
	return value != "0" && !strings.EqualFold(value, "false")
}

func GoogleBooksAPIKeyConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_KEY")) != ""
}

func (c *GoogleBooksClient) APIKey() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.apiKey)
}

func (c *GoogleBooksClient) LookupISBN(ctx context.Context, isbn13 string) (*GoogleBooksVolume, error) {
	return c.LookupISBNWithAPIKey(ctx, isbn13, c.APIKey())
}

func (c *GoogleBooksClient) LookupISBNWithAPIKey(ctx context.Context, isbn13 string, apiKey string) (*GoogleBooksVolume, error) {
	if c == nil {
		return nil, nil
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, nil
	}
	endpoint, err := url.Parse(c.baseURL + "/volumes")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("q", "isbn:"+isbn13)
	query.Set("langRestrict", "ja")
	query.Set("printType", "books")
	query.Set("maxResults", "10")
	query.Set("key", apiKey)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("google books lookup failed with HTTP %d", res.StatusCode)
	}
	var payload googleBooksVolumesResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	for _, item := range payload.Items {
		volume := item.toVolume(isbn13)
		if volume != nil {
			return volume, nil
		}
	}
	return nil, nil
}

func (c *GoogleBooksClient) HasAPIKey() bool {
	return c != nil && strings.TrimSpace(c.apiKey) != ""
}

type googleBooksVolumesResponse struct {
	Items []googleBooksItem `json:"items"`
}

type googleBooksItem struct {
	ID         string                `json:"id"`
	VolumeInfo googleBooksVolumeInfo `json:"volumeInfo"`
}

type googleBooksVolumeInfo struct {
	Title               string            `json:"title"`
	Subtitle            string            `json:"subtitle"`
	Authors             []string          `json:"authors"`
	Publisher           string            `json:"publisher"`
	PublishedDate       string            `json:"publishedDate"`
	IndustryIdentifiers []googleBooksISBN `json:"industryIdentifiers"`
	ImageLinks          map[string]string `json:"imageLinks"`
	InfoLink            string            `json:"infoLink"`
	CanonicalVolumeLink string            `json:"canonicalVolumeLink"`
}

type googleBooksISBN struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

func (item googleBooksItem) toVolume(isbn13 string) *GoogleBooksVolume {
	info := item.VolumeInfo
	if !googleBooksItemMatchesISBN(info.IndustryIdentifiers, isbn13) {
		return nil
	}
	return &GoogleBooksVolume{
		VolumeID:            strings.TrimSpace(item.ID),
		Title:               strings.TrimSpace(info.Title),
		Subtitle:            strings.TrimSpace(info.Subtitle),
		Authors:             trimNonEmptyStrings(info.Authors),
		Publisher:           strings.TrimSpace(info.Publisher),
		PublishedDate:       strings.TrimSpace(info.PublishedDate),
		ImageURL:            selectGoogleBooksImageURL(info.ImageLinks),
		InfoLink:            strings.TrimSpace(info.InfoLink),
		CanonicalVolumeLink: strings.TrimSpace(info.CanonicalVolumeLink),
	}
}

func googleBooksItemMatchesISBN(identifiers []googleBooksISBN, isbn13 string) bool {
	for _, identifier := range identifiers {
		if NormalizeISBN13(identifier.Identifier) == isbn13 {
			return true
		}
	}
	return false
}

func selectGoogleBooksImageURL(links map[string]string) string {
	if len(links) == 0 {
		return ""
	}
	for _, key := range []string{"extraLarge", "large", "medium", "small", "thumbnail", "smallThumbnail"} {
		if value := strings.TrimSpace(links[key]); value != "" {
			if strings.HasPrefix(value, "http://") {
				return strings.Replace(value, "http://", "https://", 1)
			}
			return value
		}
	}
	keys := make([]string, 0, len(links))
	for key := range links {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.TrimSpace(links[keys[0]])
}

func trimNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
