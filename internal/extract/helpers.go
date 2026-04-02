package extract

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/napkin/docs-crawler/internal/pipeline"
)

// extractTitle returns the page title, trying <title> first then falling back to first <h1>.
func extractTitle(doc *goquery.Document) string {
	if title := strings.TrimSpace(doc.Find("title").First().Text()); title != "" {
		return title
	}
	if h1 := strings.TrimSpace(doc.Find("h1").First().Text()); h1 != "" {
		return h1
	}
	return ""
}

// extractDescription returns the content of meta[name=description].
func extractDescription(doc *goquery.Document) string {
	desc, _ := doc.Find(`meta[name="description"]`).Attr("content")
	return strings.TrimSpace(desc)
}

// extractHeadings collects all h1-h6 headings from the document.
func extractHeadings(doc *goquery.Document) []pipeline.Heading {
	var headings []pipeline.Heading
	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(_ int, s *goquery.Selection) {
		tag := goquery.NodeName(s)
		level := parseHeadingLevel(tag)
		text := strings.TrimSpace(s.Text())
		if text != "" {
			headings = append(headings, pipeline.Heading{Level: level, Text: text})
		}
	})
	return headings
}

// parseHeadingLevel extracts the numeric level from a heading tag name (e.g. "h2" -> 2).
func parseHeadingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 0
}

// extractLinks collects all unique absolute href links from <a> tags.
func extractLinks(doc *goquery.Document, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var links []string

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return
		}
		resolved := resolveURL(base, href)
		if resolved == "" {
			return
		}
		if _, exists := seen[resolved]; !exists {
			seen[resolved] = struct{}{}
			links = append(links, resolved)
		}
	})

	return links
}

// resolveURL resolves a possibly-relative href against the base URL.
func resolveURL(base *url.URL, href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	resolved.Fragment = ""
	return resolved.String()
}

// countWords counts whitespace-separated tokens in the given text.
func countWords(text string) int {
	return len(strings.Fields(text))
}

// toMarkdown converts an HTML string to Markdown.
func toMarkdown(html string) (string, error) {
	md, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(md), nil
}
