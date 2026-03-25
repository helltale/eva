package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Hit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type Client struct {
	HTTP *http.Client
}

func (c *Client) Search(ctx context.Context, query string, max int) ([]Hit, error) {
	if max <= 0 {
		max = 5
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	u := "https://api.duckduckgo.com/?format=json&no_html=1&skip_disambig=1&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "EVA-LAN-Assistant/1.0")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("search status %d", res.StatusCode)
	}
	var payload struct {
		Abstract       string `json:"Abstract"`
		AbstractURL    string `json:"AbstractURL"`
		AbstractSource string `json:"AbstractSource"`
		Heading        string `json:"Heading"`
		RelatedTopics  []any  `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	var hits []Hit
	if payload.Abstract != "" {
		hits = append(hits, Hit{
			Title:   firstNonEmpty(payload.Heading, "Summary"),
			URL:     payload.AbstractURL,
			Snippet: payload.Abstract,
		})
	}
	for _, rt := range payload.RelatedTopics {
		if len(hits) >= max {
			break
		}
		switch v := rt.(type) {
		case map[string]any:
			text, _ := v["Text"].(string)
			u, _ := v["FirstURL"].(string)
			if text != "" {
				hits = append(hits, Hit{Title: trimTopic(text), URL: u, Snippet: text})
			}
		}
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("no search results (try a different query or check network)")
	}
	if len(hits) > max {
		hits = hits[:max]
	}
	return hits, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func trimTopic(s string) string {
	if idx := strings.Index(s, " - "); idx > 0 {
		return s[:idx]
	}
	return s
}

func FormatHits(hits []Hit) string {
	var b strings.Builder
	for i, h := range hits {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("%s\n%s\n%s", h.Title, h.URL, h.Snippet))
	}
	return b.String()
}
