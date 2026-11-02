package githubrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/Clementine33/preen/backend/internal/updatecheck"
)

// LatestURL is the endpoint of the latest stable release; GitHub's "latest"
// already excludes pre-releases and drafts.
const LatestURL = "https://api.github.com/repos/Clementine33/preen/releases/latest"

// maxBody bounds what is read of a release; the fields used are a few hundred
// bytes, the rest is release notes.
const maxBody = 4 << 20

// Client reads the latest release from one endpoint.
type Client struct {
	http      *http.Client
	endpoint  string
	userAgent string
}

// New reads from endpoint through client; version names the running build in
// the User-Agent. A nil client connects directly: the production path never
// reads HTTP_PROXY, HTTPS_PROXY or NO_PROXY, because the Outbound Proxy is a
// user setting (see NewProxied).
func New(client *http.Client, endpoint, version string) *Client {
	if client == nil {
		client = &http.Client{Transport: directTransport()}
	}
	return &Client{http: client, endpoint: endpoint, userAgent: "preen/" + version}
}

// NewProxied reads LatestURL through the proxy that proxy reports for each
// request's target; a nil URL means direct. An error fails the request: an
// unreadable setting must never fall back to a direct connection.
func NewProxied(proxy func(target *url.URL) (*url.URL, error), version string) *Client {
	tr := directTransport()
	tr.Proxy = func(r *http.Request) (*url.URL, error) { return proxy(r.URL) }
	return New(&http.Client{Transport: tr}, LatestURL, version)
}

func directTransport() *http.Transport {
	base, _ := http.DefaultTransport.(*http.Transport)
	tr := base.Clone()
	tr.Proxy = nil
	return tr
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Latest performs one request. Network errors, 5xx and 429 are marked
// updatecheck.Transient; 404, other 4xx and an undecodable body are not.
func (c *Client) Latest(ctx context.Context) (updatecheck.Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return updatecheck.Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return updatecheck.Release{}, updatecheck.Transient(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("release request returned status %d", resp.StatusCode)
		if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusTooManyRequests {
			err = updatecheck.Transient(err)
		}
		return updatecheck.Release{}, err
	}
	var body release
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return updatecheck.Release{}, fmt.Errorf("decode release: %w", err)
	}
	if body.TagName == "" {
		return updatecheck.Release{}, fmt.Errorf("release has no tag")
	}
	return updatecheck.Release{Tag: body.TagName, URL: body.HTMLURL}, nil
}
