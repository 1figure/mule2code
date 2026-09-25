package zippo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"muletocode/internal/contacts"
	"muletocode/internal/testutil"
)

// stubUpstream serves testdata/api/zippopotam-<country>-<code>.json at /<country>/<code>,
// 404 for unknown codes, and a few error shapes for special countries.
func stubUpstream(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		switch parts[0] {
		case "boom":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "teapot":
			w.WriteHeader(http.StatusTeapot)
		case "garbage":
			_, _ = w.Write([]byte("{not json"))
		case "badcoord":
			_, _ = w.Write([]byte(`{"places":[{"place name":"X","latitude":"north","longitude":"1"}]}`))
		default:
			data, err := os.ReadFile(testutil.FixturePath(t, "api/zippopotam-"+parts[0]+"-"+parts[1]+".json"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

// Golden: the place of results[0] in normalize-response.json is FromWire(zippopotam-us-63131.json).
func TestFromWire_Golden(t *testing.T) {
	var response struct {
		Results []struct {
			Place json.RawMessage `json:"place"`
		} `json:"results"`
	}
	testutil.ReadJSON(t, "api/normalize-response.json", &response)
	for i, code := range []string{"63131", "91499"} {
		var wire Wire
		testutil.ReadJSON(t, "api/zippopotam-us-"+code+".json", &wire)
		place, err := FromWire(wire)
		if err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(place)
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertJSONEqual(t, response.Results[i].Place, got)
	}
}

func TestFromWire_Edges(t *testing.T) {
	place, err := FromWire(Wire{})
	if err != nil || place.City != nil || place.Latitude != nil {
		t.Errorf("no places → all nil, got %+v, %v", place, err)
	}
	s := "x"
	_, err = FromWire(Wire{Places: []PlaceWire{{Latitude: &s}}})
	var ce *contacts.CoercionError
	if !errors.As(err, &ce) || err.Error() != "Cannot coerce String (x) to Number" || ce.Field != "latitude" || ce.Unwrap() == nil {
		t.Errorf("got %v", err)
	}
}

func TestLookup(t *testing.T) {
	srv, paths := stubUpstream(t)
	client := NewClient(srv.URL+"/", nil)
	ctx := context.Background()

	wire, err := client.Lookup(ctx, "US", "63131")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(wire.Places) != 1 || *wire.Places[0].PlaceName != "Saint Louis" || *wire.PostCode != "63131" {
		t.Errorf("wire = %+v", wire)
	}
	if (*paths)[0] != "/us/63131" {
		t.Errorf("country must be lower-cased in the path, got %s", (*paths)[0])
	}

	var upstream *UpstreamError
	_, err = client.Lookup(ctx, "us", "00000")
	if !errors.Is(err, ErrNotFound) || errors.As(err, &upstream) {
		t.Errorf("404: got %v, want ErrNotFound", err)
	}
	_, err = client.Lookup(ctx, "boom", "1")
	if !errors.As(err, &upstream) || !strings.Contains(err.Error(), "503 Service Unavailable") {
		t.Errorf("503: got %v", err)
	}
	_, err = client.Lookup(ctx, "garbage", "1")
	if !errors.As(err, &upstream) || !strings.Contains(err.Error(), "malformed JSON") || upstream.Unwrap() == nil {
		t.Errorf("malformed body: got %v", err)
	}
	_, err = client.Lookup(ctx, "teapot", "1")
	if err == nil || errors.As(err, &upstream) || errors.Is(err, ErrNotFound) {
		t.Errorf("418 is neither not-found nor upstream: got %v", err)
	}
	_, err = client.Lookup(ctx, "sp ace", "A B")
	if !errors.Is(err, ErrNotFound) || (*paths)[len(*paths)-1] != "/sp%20ace/A%20B" {
		t.Errorf("path escaping: %v %v", err, *paths)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = client.Lookup(cancelled, "us", "63131")
	if !errors.Is(err, context.Canceled) || errors.As(err, &upstream) {
		t.Errorf("cancelled: got %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer slow.Close()
	_, err = NewClient(slow.URL, &http.Client{Timeout: 20 * time.Millisecond}).Lookup(ctx, "us", "1")
	if !errors.As(err, &upstream) {
		t.Errorf("timeout: got %v, want *UpstreamError", err)
	}

	srv.Close()
	_, err = client.Lookup(ctx, "us", "63131")
	if !errors.As(err, &upstream) {
		t.Errorf("connection refused: got %v, want *UpstreamError", err)
	}
	if _, err := NewClient("http://[::1]:namedport", nil).Lookup(ctx, "us", "1"); err == nil {
		t.Error("an unbuildable request must be an error")
	}
}
