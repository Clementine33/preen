package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Clementine33/preen/backend/internal/works"
)

// FetchMain extracts only a main-cover URL, independent of complete Core validation.
func (c *Client) FetchMain(ctx context.Context, raw, source, locale string) (string, error) {
	id, ok := works.ParseWorkCode(raw)
	if !ok {
		return "", fmt.Errorf("invalid work code")
	}
	if locale == "" {
		locale = "ja-jp"
	}
	var data record
	field := "mainCoverUrl"
	switch source {
	case "dlsite":
		var items []record
		if err := c.get(
			ctx,
			"https://www.dlsite.com/maniax/api/=/product.json?"+url.Values{
				"workno": {requestCode(id)},
				"locale": {locale},
			}.Encode(),
			&items,
		); err != nil {
			return "", err
		}
		if len(items) != 1 {
			return "", fmt.Errorf("main cover unavailable")
		}
		data = items[0]
		actual := scalar[string](data, "workno")
		if actual == nil {
			return "", fmt.Errorf("missing work code")
		}
		code, valid := works.ParseWorkCode(*actual)
		if !valid || code != id {
			return "", fmt.Errorf("wrong cover work")
		}
		var image record
		if err := json.Unmarshal(data["image_main"], &image); err != nil {
			return "", err
		}
		data = image
		field = "url"
	case "asmr_one":
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
			return "", err
		}
		actual := scalar[string](data, "source_id")
		if actual == nil {
			return "", fmt.Errorf("missing work code")
		}
		code, valid := works.ParseWorkCode(*actual)
		if !valid || code != id {
			return "", fmt.Errorf("wrong cover work")
		}
	default:
		return "", fmt.Errorf("unsupported source")
	}
	value := cleanURL(scalar[string](data, field))
	if value == nil {
		return "", fmt.Errorf("main cover unavailable")
	}
	return *value, nil
}
