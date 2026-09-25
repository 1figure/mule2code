// Package zippo is the outbound side of normalize-contacts-flow:
// <http:request-config name="Zippopotam_Request_config"> to api.zippopotam.us on net/http.
package zippo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"muletocode/internal/contacts"
)

// ErrNotFound is the Mule HTTP:NOT_FOUND error: the service answered 404 for the postal code.
var ErrNotFound = errors.New("postal code not found")

// UpstreamError is the Mule HTTP:CONNECTIVITY, HTTP:TIMEOUT, HTTP:INTERNAL_SERVER_ERROR,
// HTTP:SERVICE_UNAVAILABLE and HTTP:BAD_GATEWAY errors, which the API maps to 502.
type UpstreamError struct {
	Msg string
	Err error
}

func (e *UpstreamError) Error() string { return e.Msg }

// Unwrap returns the transport or decoding error, if any.
func (e *UpstreamError) Unwrap() error { return e.Err }

// Wire is the answer of GET https://api.zippopotam.us/{country}/{postalCode}, as sent on the wire.
type Wire struct {
	PostCode            *string     `json:"post code"`
	Country             *string     `json:"country"`
	CountryAbbreviation *string     `json:"country abbreviation"`
	Places              []PlaceWire `json:"places"`
}

// PlaceWire is one element of Wire.Places; the coordinates are strings on the wire.
type PlaceWire struct {
	PlaceName         *string `json:"place name"`
	Longitude         *string `json:"longitude"`
	State             *string `json:"state"`
	StateAbbreviation *string `json:"state abbreviation"`
	Latitude          *string `json:"latitude"`
}

// Place is the place object of a normalize result (openapi.yaml NormalizeResponse.results[].place).
type Place struct {
	City              *string  `json:"city"`
	State             *string  `json:"state"`
	StateAbbreviation *string  `json:"state_abbreviation"`
	Latitude          *float64 `json:"latitude"`
	Longitude         *float64 `json:"longitude"`
}

// FromWire is the place part of <ee:transform doc:name="Zippopotam to result">: places[0] with
// `latitude as Number` / `longitude as Number`. An absent places[0] gives a Place of nils.
func FromWire(w Wire) (Place, error) {
	var pw PlaceWire
	if len(w.Places) > 0 {
		pw = w.Places[0]
	}
	lat, err := number(pw.Latitude, "latitude")
	if err != nil {
		return Place{}, err
	}
	lon, err := number(pw.Longitude, "longitude")
	if err != nil {
		return Place{}, err
	}
	return Place{
		City:              pw.PlaceName,
		State:             pw.State,
		StateAbbreviation: pw.StateAbbreviation,
		Latitude:          lat,
		Longitude:         lon,
	}, nil
}

func number(v *string, field string) (*float64, error) {
	if v == nil {
		return nil, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(*v), 64)
	if err != nil {
		return nil, &contacts.CoercionError{Field: field, Value: *v, Target: "Number", Err: err}
	}
	return &f, nil
}

// Client performs the lookups of normalize-contacts-flow against one base URL.
type Client struct {
	base string
	http *http.Client
}

// NewClient creates a client for baseURL (ZIPPOPOTAM_BASE_URL); a nil hc gets a 30-second timeout.
func NewClient(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), http: hc}
}

// Lookup is <http:request method="GET" path="/{country}/{postalCode}">; country is lower-cased in
// the path as the uri-params expression did. A 404 returns an error wrapping ErrNotFound; a
// connection failure, a timeout, a 500/502/503 answer or a malformed body returns an
// *UpstreamError; any other non-2xx status is a plain error.
func (c *Client) Lookup(ctx context.Context, country, postalCode string) (Wire, error) {
	u := c.base + "/" + url.PathEscape(strings.ToLower(country)) + "/" + url.PathEscape(postalCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Wire{}, fmt.Errorf("building request for %s: %w", u, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Wire{}, fmt.Errorf("HTTP GET on resource '%s': %w", u, ctx.Err())
		}
		return Wire{}, &UpstreamError{Msg: fmt.Sprintf("HTTP GET on resource '%s' failed: %v", u, err), Err: err}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return Wire{}, fmt.Errorf("%s/%s: %w", country, postalCode, ErrNotFound)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return Wire{}, &UpstreamError{Msg: fmt.Sprintf("HTTP GET on resource '%s' failed: %d %s", u, resp.StatusCode, http.StatusText(resp.StatusCode))}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Wire{}, fmt.Errorf("HTTP GET on resource '%s' failed: %d %s", u, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	var w Wire
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&w); err != nil {
		return Wire{}, &UpstreamError{Msg: fmt.Sprintf("HTTP GET on resource '%s' returned malformed JSON: %v", u, err), Err: err}
	}
	return w, nil
}
