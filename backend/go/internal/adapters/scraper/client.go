package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

// Client retrieves metadata from the two fixed source hosts.
type Client struct {
	http   *http.Client
	dlsite *sourceGate
	asmr   *sourceGate
}

// New accepts a transport for local tests; production uses the standard HTTP client.
func New(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	if copyClient.Timeout == 0 {
		copyClient.Timeout = 10 * time.Second
	}
	if copyClient.Jar == nil {
		copyClient.Jar, _ = cookiejar.New(nil)
	}
	previous := copyClient.CheckRedirect
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("upstream redirect leaves its source")
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return &Client{http: &copyClient, dlsite: newSourceGate(8, time.Second/10), asmr: newSourceGate(2, time.Second/3)}
}

// requestCode is the upstream spelling, independent of the canonical catalog key.
func requestCode(id works.WorkCode) string {
	digits := strings.TrimLeft(strings.TrimPrefix(string(id), "RJ"), "0")
	if len(digits) <= 6 {
		return "RJ" + strings.Repeat("0", 6-len(digits)) + digits
	}
	return string(id)
}

// FetchStatic reads only this work's basic description and edition report.
func (c *Client) FetchStatic(ctx context.Context, id works.WorkCode, locale string) (refresh.Snapshot, error) {
	var items []record
	query := url.Values{"workno": {requestCode(id)}, "locale": {locale}}
	if err := c.get(ctx, "https://www.dlsite.com/maniax/api/=/product.json?"+query.Encode(), &items); err != nil {
		return refresh.Snapshot{}, err
	}
	if len(items) != 1 {
		return refresh.Snapshot{}, fmt.Errorf("static work is unavailable")
	}
	actual := scalar[string](items[0], "workno")
	if actual == nil {
		return refresh.Snapshot{}, fmt.Errorf("static response has no work number")
	}
	code, ok := works.ParseWorkCode(*actual)
	if !ok || code != id {
		return refresh.Snapshot{}, fmt.Errorf("static response belongs to another work")
	}
	var image record
	_ = json.Unmarshal(items[0]["image_main"], &image)
	main := ""
	if value := cleanURL(scalar[string](image, "url")); value != nil {
		main = *value
	}
	return refresh.Snapshot{
		ImportRequest: validSnapshot(id, dlsiteSnapshot(items[0], time.Now().UTC().Format(works.TimestampFormat))),
		MainURL:       main,
	}, nil
}

// FetchDynamic reads a stable batch; missing keys remain absent from the result.
func (c *Client) FetchDynamic(
	ctx context.Context,
	ids []works.WorkCode,
	locale string,
) (map[works.WorkCode]works.ImportRequest, error) {
	codes := make([]string, 0, len(ids))
	for _, id := range ids {
		codes = append(codes, requestCode(id))
	}
	query := url.Values{"product_id": {strings.Join(codes, ",")}, "locale": {locale}}
	var records map[string]record
	if err := c.get(
		ctx,
		"https://www.dlsite.com/maniax-touch/product/info/ajax?"+query.Encode(),
		&records,
	); err != nil {
		var status *httpError
		if errors.As(err, &status) && status.code == http.StatusRequestURITooLong && len(ids) > 1 {
			first, splitErr := c.FetchDynamic(ctx, ids[:len(ids)/2], locale)
			if splitErr != nil {
				return nil, splitErr
			}
			second, splitErr := c.FetchDynamic(ctx, ids[len(ids)/2:], locale)
			if splitErr != nil {
				return nil, splitErr
			}
			maps.Copy(first, second)
			return first, nil
		}
		return nil, err
	}
	return decodeDynamic(ids, records, time.Now().UTC().Format(works.TimestampFormat))
}

func decodeDynamic(
	ids []works.WorkCode,
	records map[string]record,
	at string,
) (map[works.WorkCode]works.ImportRequest, error) {
	out := map[works.WorkCode]works.ImportRequest{}
	for raw, data := range records {
		id, ok := works.ParseWorkCode(raw)
		if !ok {
			return nil, fmt.Errorf("invalid response work number %q", raw)
		}
		requested := slices.Contains(ids, id)
		if !requested {
			return nil, fmt.Errorf("unrequested response work number %q", raw)
		}
		snap := works.ImportRequest{Relations: relations(data, "dlsite", at)}
		if count, present := data["dl_count"]; present {
			var n *int
			decodeErr := json.Unmarshal(count, &n)
			if decodeErr != nil {
				var text string
				if json.Unmarshal(count, &text) == nil {
					number, err := strconv.Atoi(text)
					if err == nil {
						n = &number
						decodeErr = nil
					}
				}
			}
			if decodeErr == nil {
				snap.Statistics = &works.ImportStatistics{FetchedAt: at, DLCount: n}
			}
		}
		out[id] = validSnapshot(id, snap)
	}
	return out, nil
}

// FetchASMR is implemented separately from DLsite's independent channels.
func (c *Client) FetchASMR(ctx context.Context, id works.WorkCode, locale string) (refresh.Snapshot, error) {
	var data record
	if err := c.get(
		ctx,
		"https://api.asmr.one/api/workInfo/"+strings.TrimLeft(
			strings.TrimPrefix(string(id), "RJ"),
			"0",
		)+"?"+url.Values{
			"locale": {locale},
		}.Encode(),
		&data,
	); err != nil {
		return refresh.Snapshot{}, err
	}
	raw := scalar[string](data, "source_id")
	if raw == nil {
		return refresh.Snapshot{}, fmt.Errorf("backup response has no work number")
	}
	actual, ok := works.ParseWorkCode(*raw)
	if !ok || actual != id {
		return refresh.Snapshot{}, fmt.Errorf("backup response belongs to another work")
	}
	main := ""
	if value := cleanURL(scalar[string](data, "mainCoverUrl")); value != nil {
		main = *value
	}
	return refresh.Snapshot{
		ImportRequest: validSnapshot(id, asmrSnapshot(data, time.Now().UTC().Format(works.TimestampFormat))),
		MainURL:       main,
	}, nil
}
