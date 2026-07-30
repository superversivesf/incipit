package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jason/incipit/internal/models"
)

type OLClient struct {
	baseURL string
	http    *http.Client
}

func NewOLClient(baseURL string) *OLClient {
	return &OLClient{
		baseURL: baseURL,
		http:    &http.Client{},
	}
}

func (c *OLClient) LookupByISBN(ctx context.Context, isbn string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/api/books?bibkeys=ISBN:%s&format=json&jscmd=data", c.baseURL, url.QueryEscape(isbn))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseOLResponse(body)
}

func (c *OLClient) LookupByTitle(ctx context.Context, title, author string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/search.json?title=%s&author=%s&limit=1",
		c.baseURL, url.QueryEscape(title), url.QueryEscape(author))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseOLSearchResponse(body)
}

func (c *OLClient) get(ctx context.Context, url string) ([]byte, error) {
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

type olISBNResponse map[string]struct {
	Title   string `json:"title"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	PublishDate   string `json:"publish_date"`
	NumberOfPages int    `json:"number_of_pages"`
	Subjects      []struct {
		Name string `json:"name"`
	} `json:"subjects"`
	Cover struct {
		Large string `json:"large"`
	} `json:"cover"`
}

type olSearchResponse struct {
	NumFound int `json:"numFound"`
	Docs     []struct {
		Title               string   `json:"title"`
		AuthorName          []string `json:"author_name"`
		FirstPublishYear    int      `json:"first_publish_year"`
		ISBN                []string `json:"isbn"`
		Subject             []string `json:"subject"`
		CoverI              int      `json:"cover_i"`
		NumberOfPagesMedian int      `json:"number_of_pages_median"`
	} `json:"docs"`
}

func ParseOLResponse(data []byte) (*models.LookupResult, error) {
	var resp olISBNResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing Open Library response: %w", err)
	}

	for _, book := range resp {
		return olBookToResult(book), nil
	}
	return nil, nil
}

func ParseOLSearchResponse(data []byte) (*models.LookupResult, error) {
	var resp olSearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing Open Library search response: %w", err)
	}

	if len(resp.Docs) == 0 {
		return nil, nil
	}

	doc := resp.Docs[0]
	result := &models.LookupResult{
		Title:     doc.Title,
		Pages:     doc.NumberOfPagesMedian,
		Published: fmt.Sprintf("%d", doc.FirstPublishYear),
		Sources:   []string{"openlibrary"},
	}

	if len(doc.AuthorName) > 0 {
		result.Author = strings.Join(doc.AuthorName, ", ")
	}
	if len(doc.ISBN) > 0 {
		result.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-L.jpg", doc.ISBN[0])
	}
	if doc.CoverI > 0 {
		result.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
	}

	for _, s := range doc.Subject {
		if strings.HasPrefix(s, "series:") {
			result.Series = strings.TrimPrefix(s, "series:")
		} else {
			result.Subjects = append(result.Subjects, s)
		}
	}

	return result, nil
}

func olBookToResult(book struct {
	Title   string `json:"title"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	PublishDate   string `json:"publish_date"`
	NumberOfPages int    `json:"number_of_pages"`
	Subjects      []struct {
		Name string `json:"name"`
	} `json:"subjects"`
	Cover struct {
		Large string `json:"large"`
	} `json:"cover"`
}) *models.LookupResult {
	result := &models.LookupResult{
		Title:     book.Title,
		Pages:     book.NumberOfPages,
		Published: book.PublishDate,
		Sources:   []string{"openlibrary"},
	}

	if len(book.Authors) > 0 {
		result.Author = book.Authors[0].Name
	}
	if len(book.Publishers) > 0 {
		result.Publisher = book.Publishers[0].Name
	}
	if book.Cover.Large != "" {
		result.CoverURL = book.Cover.Large
	}

	for _, s := range book.Subjects {
		if strings.HasPrefix(s.Name, "series:") {
			result.Series = strings.TrimPrefix(s.Name, "series:")
		} else {
			result.Subjects = append(result.Subjects, s.Name)
		}
	}

	return result
}
