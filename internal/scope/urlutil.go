package scope

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// NormalizeURL returns a canonical string representation of a URL suitable for
// deduplication. It lowercases the host, removes the fragment, strips trailing
// slashes from the path, and sorts query parameters.
func NormalizeURL(u *url.URL) string {
	normalized := &url.URL{
		Scheme:   strings.ToLower(u.Scheme),
		Host:     strings.ToLower(u.Host),
		Path:     strings.TrimRight(u.Path, "/"),
		RawQuery: sortQuery(u.Query()),
	}

	if normalized.Path == "" {
		normalized.Path = "/"
	}

	return normalized.String()
}

// sortQuery encodes query values with keys in sorted order.
func sortQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, k := range keys {
		vals := values[k]
		sort.Strings(vals)
		for j, v := range vals {
			if i > 0 || j > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(url.QueryEscape(k))
			buf.WriteByte('=')
			buf.WriteString(url.QueryEscape(v))
		}
	}

	return buf.String()
}

// URLToFilepath converts a page URL to a relative file path for output.
// The base URL is used to strip the common prefix. For example:
//
//	base: "https://docs.example.com"
//	page: "https://docs.example.com/api/auth"
//	result: "api/auth"
//
// Edge cases:
//   - Root path returns "index"
//   - Paths ending in "/" get "index" appended
func URLToFilepath(baseURL, pageURL string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL %q: %w", baseURL, err)
	}

	page, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("parsing page URL %q: %w", pageURL, err)
	}

	// Strip the base path prefix from the page path.
	basePath := strings.TrimRight(base.Path, "/")
	pagePath := page.Path

	rel := strings.TrimPrefix(pagePath, basePath)
	rel = strings.TrimPrefix(rel, "/")

	// Clean the path to remove double slashes, dots, etc.
	rel = path.Clean(rel)

	if rel == "" || rel == "." || rel == "/" {
		return "index", nil
	}

	if strings.HasSuffix(pagePath, "/") {
		rel = path.Join(rel, "index")
	}

	return rel, nil
}
