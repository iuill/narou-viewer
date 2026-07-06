package publications

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const ndlProviderID = "ndl_search"

type NDLClient struct {
	baseURL string
	client  *http.Client
}

func NewNDLClientFromEnv() *NDLClient {
	baseURL := strings.TrimSpace(os.Getenv("NDL_SEARCH_API_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://ndlsearch.ndl.go.jp/api/opensearch"
	}
	return &NDLClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func NDLSearchEnabled() bool {
	value := strings.TrimSpace(os.Getenv("PUBLICATION_PROVIDER_NDL_ENABLED"))
	return value != "0" && !strings.EqualFold(value, "false")
}

func (c *NDLClient) LookupISBN(ctx context.Context, isbn13 string) (*NDLBibliography, error) {
	if c == nil {
		return nil, nil
	}
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("isbn", isbn13)
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
		return nil, fmt.Errorf("NDL search lookup failed with HTTP %d", res.StatusCode)
	}
	return parseNDLOpenSearch(res.Body, isbn13)
}

type ndlItem struct {
	Title       string
	Link        string
	Creators    []string
	Publishers  []string
	Dates       []string
	Identifiers []string
}

func parseNDLOpenSearch(reader io.Reader, isbn13 string) (*NDLBibliography, error) {
	decoder := xml.NewDecoder(reader)
	var current *ndlItem
	var currentElement string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			name := typed.Name.Local
			if name == "item" {
				current = &ndlItem{}
				currentElement = ""
				continue
			}
			if current != nil {
				currentElement = name
			}
		case xml.EndElement:
			if typed.Name.Local == "item" && current != nil {
				if bibliography := current.toBibliography(isbn13); bibliography != nil {
					return bibliography, nil
				}
				current = nil
			}
			currentElement = ""
		case xml.CharData:
			if current != nil && currentElement != "" {
				current.addValue(currentElement, string(typed))
			}
		}
	}
	return nil, nil
}

func (item *ndlItem) addValue(element string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	switch element {
	case "title":
		if item.Title == "" {
			item.Title = value
		}
	case "link":
		if item.Link == "" {
			item.Link = value
		}
	case "creator":
		item.Creators = appendIfMissing(item.Creators, value)
	case "publisher":
		item.Publishers = appendIfMissing(item.Publishers, value)
	case "date", "issued":
		item.Dates = appendIfMissing(item.Dates, value)
	case "identifier":
		item.Identifiers = appendIfMissing(item.Identifiers, value)
	}
}

func (item ndlItem) toBibliography(isbn13 string) *NDLBibliography {
	if !item.matchesISBN(isbn13) {
		return nil
	}
	return &NDLBibliography{
		Title:         strings.TrimSpace(item.Title),
		Authors:       trimNonEmptyStrings(item.Creators),
		Publisher:     firstNonEmpty(item.Publishers...),
		PublishedDate: firstNonEmpty(item.Dates...),
		DetailURL:     strings.TrimSpace(item.Link),
	}
}

func (item ndlItem) matchesISBN(isbn13 string) bool {
	if len(item.Identifiers) == 0 {
		return false
	}
	for _, identifier := range item.Identifiers {
		if NormalizeISBN13(identifier) == isbn13 {
			return true
		}
	}
	return false
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
