// Package github is a minimal client for the GitHub Releases API.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://api.github.com"
	defaultUA       = "youtube-dl-gui"
	apiTimeout      = 5 * time.Second
	downloadTimeout = 30 * time.Minute
)

var (
	// ErrRateLimited is returned when the GitHub API rate limit is exhausted.
	ErrRateLimited = errors.New("github: rate limited")
	// ErrOffline is returned on network-level failures (DNS, connection, timeout).
	ErrOffline = errors.New("github: offline")
	// ErrNotFound is returned on HTTP 404.
	ErrNotFound = errors.New("github: not found")
)

// Client talks to the GitHub API. The zero value is usable.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

// Asset is a file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Release is a GitHub release.
type Release struct {
	TagName     string  `json:"tag_name"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

// Asset returns the asset with the exact given name.
func (r Release) Asset(name string) (Asset, bool) {
	return r.AssetMatch(func(n string) bool { return n == name })
}

// AssetMatch returns the first asset whose name satisfies match.
func (r Release) AssetMatch(match func(string) bool) (Asset, bool) {
	for _, a := range r.Assets {
		if match(a.Name) {
			return a, true
		}
	}
	return Asset{}, false
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultBaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return defaultUA
}

// wrapNetErr maps transport errors to ErrOffline, leaving context
// cancellation untouched.
func wrapNetErr(ctx context.Context, err error) error {
	if ctx.Err() != nil && !errors.Is(err, context.DeadlineExceeded) {
		return ctx.Err()
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrOffline, err)
	}
	return fmt.Errorf("%w: %v", ErrOffline, err)
}

func statusErr(resp *http.Response) error {
	switch {
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		resp.Header.Get("X-RateLimit-Remaining") == "0":
		return ErrRateLimited
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("github: unexpected status %s", resp.Status)
	}
}

// LatestRelease fetches the latest release of owner/repo (5 s timeout).
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL(), owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Release{}, wrapNetErr(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, statusErr(resp)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decode release: %w", err)
	}
	return rel, nil
}

// Download fetches url into dst. Data is written to dst+".part" and renamed
// on success, so dst is never left partially written. progress may be nil;
// total is -1 when unknown.
func (c *Client) Download(ctx context.Context, url, dst string, progress func(done, total int64)) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return wrapNetErr(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusErr(resp)
	}

	part := dst + ".part"
	f, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("create %s: %w", part, err)
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(part)
		}
	}()

	var done int64
	total := resp.ContentLength
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write %s: %w", part, werr)
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return wrapNetErr(ctx, rerr)
		}
	}
	if total >= 0 && done != total {
		return fmt.Errorf("download truncated: got %d of %d bytes", done, total)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", part, err)
	}
	if err := os.Rename(part, dst); err != nil {
		return fmt.Errorf("rename %s: %w", part, err)
	}
	ok = true
	return nil
}
