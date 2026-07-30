package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jason/incipit/internal/models"
)

type GBClient struct {
	baseURL string
	http    *http.Client
}

func NewGBClient(baseURL string) *GBClient {
	return &GBClient{
		baseURL: baseURL,
		http:    &http.Client{},
	}
}

func (c *GBClient) LookupByISBN(ctx context.Context, isbn string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/books/v1/volumes?q=isbn:%s", c.baseURL, url.QueryEscape(isbn))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseGBResponse(body)
}

func (c *GBClient) LookupByTitle(ctx context.Context, title, author string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/books/v1/volumes?q=intitle:%s+inauthor:%s",
		c.baseURL, url.QueryEscape(title), url.QueryEscape(author))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseGBResponse(body)
}

func (c *GBClient) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "incipit/0.1 (https://github.com/superversivesf/incipit)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return body, nil
}

type gbResponse struct {
	Items []struct {
		VolumeInfo struct {
			Title         string   `json:"title"`
			Authors       []string `json:"authors"`
			PublishedDate string   `json:"publishedDate"`
			Description   string   `json:"description"`
			Categories    []string `json:"categories"`
			AverageRating float64  `json:"averageRating"`
			ImageLinks    struct {
				Thumbnail string `json:"thumbnail"`
			} `json:"imageLinks"`
		} `json:"volumeInfo"`
	} `json:"items"`
}

func ParseGBResponse(data []byte) (*models.LookupResult, error) {
	var resp gbResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing Google Books response: %w", err)
	}

	if len(resp.Items) == 0 {
		return nil, nil
	}

	vi := resp.Items[0].VolumeInfo
	result := &models.LookupResult{
		Title:       vi.Title,
		Published:   vi.PublishedDate,
		Description: vi.Description,
		Rating:      vi.AverageRating,
		Subjects:    vi.Categories,
		Sources:     []string{"googlebooks"},
	}

	if len(vi.Authors) > 0 {
		result.Author = vi.Authors[0]
	}
	if vi.ImageLinks.Thumbnail != "" {
		result.CoverURL = vi.ImageLinks.Thumbnail
	}

	return result, nil
}
