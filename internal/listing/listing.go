// Package listing reads HTTP directory listings (such as Apache's autoindex)
// and returns the sub-directories and .pkg files they link to.
package listing

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// UnknownSize marks an entry whose size has not been resolved.
const UnknownSize int64 = -1

// Entry is a link found in a directory listing.
type Entry struct {
	Name  string // display name, unescaped, without trailing slash
	URL   string // absolute URL
	IsDir bool
	Size  int64 // bytes, or UnknownSize
}

// Client fetches listings and file sizes.
type Client struct {
	HTTP *http.Client
	// StatConcurrency limits parallel HEAD requests in Sizes.
	StatConcurrency int
}

// New returns a Client using the given HTTP client.
func New(c *http.Client) *Client {
	return &Client{HTTP: c, StatConcurrency: 4}
}

var hrefRE = regexp.MustCompile(`(?i)<a\s[^>]*href\s*=\s*"([^"]+)"`)

// List fetches dirURL and returns its sub-directories followed by its .pkg
// files, each group sorted by name.
func (c *Client) List(ctx context.Context, dirURL string) ([]Entry, error) {
	base, err := url.Parse(EnsureDirURL(dirURL))
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("URL deve começar com http:// ou https://")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listagem retornou HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return Parse(base, string(body)), nil
}

// Parse extracts entries from an HTML listing of base. Only links that point
// to direct children of base are kept, which drops parent and sort links.
func Parse(base *url.URL, body string) []Entry {
	seen := map[string]bool{}
	var dirs, files []Entry

	for _, m := range hrefRE.FindAllStringSubmatch(body, -1) {
		ref, err := url.Parse(html.UnescapeString(m[1]))
		if err != nil || ref.RawQuery != "" || ref.Fragment != "" {
			continue
		}
		abs := base.ResolveReference(ref)
		if abs.Host != base.Host || !strings.HasPrefix(abs.Path, base.Path) {
			continue
		}
		rel := strings.TrimPrefix(abs.Path, base.Path)
		isDir := strings.HasSuffix(rel, "/")
		name := strings.TrimSuffix(rel, "/")
		if name == "" || strings.Contains(name, "/") {
			continue // the directory itself, or not a direct child
		}
		if !isDir && !strings.EqualFold(path.Ext(name), ".pkg") {
			continue
		}
		key := abs.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		e := Entry{Name: name, URL: key, IsDir: isDir, Size: UnknownSize}
		if isDir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	byName := func(s []Entry) {
		sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i].Name) < strings.ToLower(s[j].Name) })
	}
	byName(dirs)
	byName(files)
	return append(dirs, files...)
}

// Size returns the Content-Length of fileURL using a HEAD request.
func (c *Client) Size(ctx context.Context, fileURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, nil)
	if err != nil {
		return UnknownSize, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return UnknownSize, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UnknownSize, fmt.Errorf("HEAD retornou HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength < 0 {
		return UnknownSize, fmt.Errorf("servidor não informou o tamanho")
	}
	return resp.ContentLength, nil
}

// Sizes fills in the size of every file entry in place. Errors are ignored and
// leave the size as UnknownSize.
func (c *Client) Sizes(ctx context.Context, entries []Entry) {
	limit := c.StatConcurrency
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := range entries {
		if entries[i].IsDir {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(e *Entry) {
			defer wg.Done()
			defer func() { <-sem }()
			if n, err := c.Size(ctx, e.URL); err == nil {
				e.Size = n
			}
		}(&entries[i])
	}
	wg.Wait()
}

// EnsureDirURL appends a trailing slash so relative links resolve inside the
// directory.
func EnsureDirURL(u string) string {
	u = strings.TrimSpace(u)
	if u != "" && !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u
}

// Parent returns the URL of the parent directory, or ok=false at the root.
func Parent(dirURL string) (string, bool) {
	u, err := url.Parse(EnsureDirURL(dirURL))
	if err != nil || u.Path == "/" || u.Path == "" {
		return dirURL, false
	}
	u.Path = path.Dir(strings.TrimSuffix(u.Path, "/"))
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawPath = ""
	return u.String(), true
}
