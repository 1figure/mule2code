package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muletocode/internal/api"
	"muletocode/internal/config"
	"muletocode/internal/contacts"
	"muletocode/internal/db"
	"muletocode/internal/testutil"
	"muletocode/internal/zippo"
)

// stubLookup replaces the upstream client with the recorded answers in testdata/api.
type stubLookup struct {
	t     *testing.T
	calls []string
	fail  error
}

func (s *stubLookup) Lookup(_ context.Context, country, postalCode string) (zippo.Wire, error) {
	s.calls = append(s.calls, strings.ToLower(country)+"/"+postalCode)
	if s.fail != nil {
		return zippo.Wire{}, s.fail
	}
	data, err := os.ReadFile(testutil.FixturePath(s.t, "api/zippopotam-"+strings.ToLower(country)+"-"+postalCode+".json"))
	if err != nil {
		return zippo.Wire{}, zippo.ErrNotFound
	}
	var w zippo.Wire
	if err := json.Unmarshal(data, &w); err != nil {
		s.t.Fatal(err)
	}
	return w, nil
}

// failingRepo stands in for a database that is down.
type failingRepo struct{ err error }

func (r failingRepo) QueryByCity(context.Context, string, int) ([]db.ContactRow, error) {
	return nil, r.err
}

type panickingRepo struct{}

func (panickingRepo) QueryByCity(context.Context, string, int) ([]db.ContactRow, error) {
	panic("boom")
}

// seededRepo is a SQLite repository loaded with contact-data-100.csv.
func seededRepo(t *testing.T) *db.Repository {
	t.Helper()
	repo, err := db.Open(context.Background(), config.SQLite, filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	f, err := os.Open(testutil.Fixture(t, "contact-data-100.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var all []contacts.Contact
	for row, err := range contacts.ReadRows(f) {
		if err != nil {
			t.Fatal(err)
		}
		c, err := contacts.FromRow(row, contacts.PrecisionDate)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, c)
	}
	if err := repo.BulkInsert(context.Background(), all); err != nil {
		t.Fatal(err)
	}
	return repo
}

type host struct {
	srv  *httptest.Server
	stub *stubLookup
}

func newHost(t *testing.T, repo api.Repository) *host {
	t.Helper()
	stub := &stubLookup{t: t}
	srv := httptest.NewServer(api.NewHandler(repo, stub, 50, nil))
	t.Cleanup(srv.Close)
	return &host{srv: srv, stub: stub}
}

func (h *host) get(t *testing.T, query string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(h.srv.URL + "/contacts" + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	return resp.StatusCode, body
}

func (h *host) post(t *testing.T, body []byte) (int, []byte) {
	t.Helper()
	resp, err := http.Post(h.srv.URL+"/contacts/normalize", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func errorOf(t *testing.T, body []byte) string {
	t.Helper()
	var e struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.Error == nil {
		t.Fatalf("not an error body: %s", body)
	}
	return *e.Error
}

func TestGetContacts(t *testing.T) {
	h := newHost(t, seededRepo(t))

	t.Run("golden", func(t *testing.T) {
		status, body := h.get(t, "?city=Saint%20Louis")
		if status != http.StatusOK {
			t.Fatalf("status %d: %s", status, body)
		}
		testutil.AssertJSONEqual(t, testutil.ReadFixture(t, "api/get-contacts-saint-louis.json"), body)
	})
	t.Run("city is case-insensitive", func(t *testing.T) {
		_, body := h.get(t, "?city=saint%20louis")
		var v struct {
			City  string `json:"city"`
			Count int    `json:"count"`
		}
		if err := json.Unmarshal(body, &v); err != nil || v.Count != 2 || v.City != "saint louis" {
			t.Errorf("got %s", body)
		}
	})
	t.Run("limit is honoured", func(t *testing.T) {
		_, body := h.get(t, "?city=Saint%20Louis&limit=1")
		var v struct {
			Count    int `json:"count"`
			Contacts []struct {
				LastName string `json:"last_name"`
			} `json:"contacts"`
		}
		if err := json.Unmarshal(body, &v); err != nil || v.Count != 1 || len(v.Contacts) != 1 || v.Contacts[0].LastName != "Hugenin" {
			t.Errorf("got %s", body)
		}
	})
	t.Run("unknown city returns an empty array, not null", func(t *testing.T) {
		_, body := h.get(t, "?city=Atlantis")
		if !strings.Contains(string(body), `"contacts":[]`) {
			t.Errorf("got %s", body)
		}
	})

	// <validation:is-not-blank-string> and <validation:is-number minValue=1 maxValue=500> → 400
	for query, want := range map[string]string{
		"":                              api.ErrCityRequired.Error(),
		"?city=":                        api.ErrCityRequired.Error(),
		"?city=%20%20":                  api.ErrCityRequired.Error(),
		"?city=Saint%20Louis&limit=0":   api.ErrLimitRange.Error(),
		"?city=Saint%20Louis&limit=501": api.ErrLimitRange.Error(),
		"?city=Saint%20Louis&limit=-1":  api.ErrLimitRange.Error(),
		"?city=Saint%20Louis&limit=abc": api.ErrLimitRange.Error(),
		"?city=Saint%20Louis&limit=1.5": api.ErrLimitRange.Error(),
	} {
		t.Run("400 "+query, func(t *testing.T) {
			status, body := h.get(t, query)
			if status != http.StatusBadRequest || errorOf(t, body) != want {
				t.Errorf("got %d %s, want 400 %q", status, body, want)
			}
		})
	}
	if api.ErrCityRequired.Error() != "Query parameter 'city' is required" || api.ErrLimitRange.Error() != "Query parameter 'limit' must be between 1 and 500" {
		t.Error("validation messages must be the XML's")
	}

	t.Run("limit 500 and the default 50 are accepted", func(t *testing.T) {
		if status, _ := h.get(t, "?city=x&limit=500"); status != http.StatusOK {
			t.Errorf("limit=500 → %d", status)
		}
		if limit, err := api.ValidateQuery("x", ""); err != nil || limit != 50 {
			t.Errorf("default limit = %d, %v", limit, err)
		}
	})

	t.Run("database error → 503", func(t *testing.T) {
		down := newHost(t, failingRepo{&db.Error{Op: "Select contacts by city failed", Err: errors.New("connection refused")}})
		status, body := down.get(t, "?city=x")
		if status != http.StatusServiceUnavailable || errorOf(t, body) != "Select contacts by city failed: connection refused" {
			t.Errorf("got %d %s", status, body)
		}
	})
	t.Run("other error → 500", func(t *testing.T) {
		down := newHost(t, failingRepo{errors.New("weird")})
		if status, body := down.get(t, "?city=x"); status != http.StatusInternalServerError || errorOf(t, body) != "weird" {
			t.Errorf("got %d %s", status, body)
		}
		broken := newHost(t, panickingRepo{})
		if status, body := broken.get(t, "?city=x"); status != http.StatusInternalServerError || errorOf(t, body) != "boom" {
			t.Errorf("panic: got %d %s", status, body)
		}
	})
	t.Run("wrong method → 405", func(t *testing.T) {
		resp, err := http.Post(h.srv.URL+"/contacts?city=x", "text/plain", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("got %d", resp.StatusCode)
		}
	})
}

func TestNormalizeContacts(t *testing.T) {
	h := newHost(t, seededRepo(t))

	t.Run("golden", func(t *testing.T) {
		h.stub.calls = nil
		status, body := h.post(t, testutil.ReadFixture(t, "api/normalize-request.json"))
		if status != http.StatusOK {
			t.Fatalf("status %d: %s", status, body)
		}
		testutil.AssertJSONEqual(t, testutil.ReadFixture(t, "api/normalize-response.json"), body)
		if want := []string{"us/63131", "us/91499", "us/00000"}; strings.Join(h.stub.calls, " ") != strings.Join(want, " ") {
			t.Errorf("upstream calls = %v, want %v (one per contact, in order, country lower-cased)", h.stub.calls, want)
		}
	})

	many := make([]map[string]string, 51)
	for i := range many {
		many[i] = map[string]string{"mailing_postal_code": "63131", "mailing_country": "US"}
	}
	manyBody, _ := json.Marshal(map[string]any{"contacts": many})

	// The validations of the XML → 400, each checked before any upstream call.
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{"fixture normalize-request-invalid.json", testutil.ReadFixture(t, "api/normalize-request-invalid.json"), api.ErrPostalCodeRequired.Error()},
		{"empty object", []byte(`{}`), api.ErrContactsPresent.Error()},
		{"empty body (payload.contacts default [])", nil, api.ErrContactsPresent.Error()},
		{"empty array", []byte(`{"contacts": []}`), api.ErrContactsPresent.Error()},
		{"null contacts", []byte(`{"contacts": null}`), api.ErrContactsPresent.Error()},
		{"too many", manyBody, api.TooManyContactsError(50).Error()},
		{"missing country", []byte(`{"contacts": [{"mailing_postal_code": "63131", "mailing_country": " "}]}`), api.ErrCountryRequired.Error()},
		{"missing both (validation:all)", []byte(`{"contacts": [{"contact_id": "x"}]}`), api.ErrPostalCodeRequired.Error() + "\n" + api.ErrCountryRequired.Error()},
		{"one bad among good", []byte(`{"contacts": [{"mailing_postal_code": "63131", "mailing_country": "US"}, {"mailing_postal_code": "", "mailing_country": "US"}]}`), api.ErrPostalCodeRequired.Error()},
	}
	for _, tc := range cases {
		t.Run("400 "+tc.name, func(t *testing.T) {
			h.stub.calls = nil
			status, body := h.post(t, tc.body)
			if status != http.StatusBadRequest || errorOf(t, body) != tc.want {
				t.Errorf("got %d %s, want 400 %q", status, body, tc.want)
			}
			if len(h.stub.calls) != 0 {
				t.Errorf("validation must happen before any upstream call, saw %v", h.stub.calls)
			}
		})
	}
	t.Run("400 invalid JSON", func(t *testing.T) {
		h.stub.calls = nil
		status, body := h.post(t, []byte(`not json`))
		if status != http.StatusBadRequest || !strings.HasPrefix(errorOf(t, body), "Invalid JSON body: ") || len(h.stub.calls) != 0 {
			t.Errorf("got %d %s", status, body)
		}
		if status, _ := h.post(t, []byte(`{"contacts": "nope"}`)); status != http.StatusBadRequest {
			t.Errorf("contacts of the wrong type → %d", status)
		}
	})
	for _, msg := range []string{api.ErrContactsPresent.Error(), api.ErrPostalCodeRequired.Error(), api.ErrCountryRequired.Error(), api.TooManyContactsError(50).Error()} {
		if !map[string]bool{
			"Body must contain a non-empty 'contacts' array": true,
			"Every contact needs 'mailing_postal_code'":      true,
			"Every contact needs 'mailing_country'":          true,
			"At most 50 contacts per request":                true,
		}[msg] {
			t.Errorf("validation message %q is not the XML's", msg)
		}
	}

	t.Run("upstream failure → 502", func(t *testing.T) {
		h.stub.fail = &zippo.UpstreamError{Msg: "HTTP GET on resource 'https://api.zippopotam.us/us/63131' failed: 503 Service Unavailable"}
		defer func() { h.stub.fail = nil }()
		status, body := h.post(t, testutil.ReadFixture(t, "api/normalize-request.json"))
		// {error: error.description}: the "Upstream error: " prefix is in the <logger> only.
		if status != http.StatusBadGateway || errorOf(t, body) != h.stub.fail.Error() {
			t.Errorf("got %d %s", status, body)
		}
	})
	t.Run("non-JSON content type → 400", func(t *testing.T) {
		h.stub.calls = nil
		resp, err := http.Post(h.srv.URL+"/contacts/normalize", "text/plain", strings.NewReader("contacts=1"))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusBadRequest || errorOf(t, body) != "Request body must be application/json" || len(h.stub.calls) != 0 {
			t.Errorf("got %d %s", resp.StatusCode, body)
		}
		// A +json media type and a charset parameter are fine.
		resp, err = http.Post(h.srv.URL+"/contacts/normalize", "application/vnd.api+json; charset=utf-8", bytes.NewReader(testutil.ReadFixture(t, "api/normalize-request.json")))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("+json: got %d", resp.StatusCode)
		}
	})
	t.Run("places: null → ok with null place fields", func(t *testing.T) {
		// `payload.places[0]` is null in DataWeave when the array is missing; the result is still status "ok".
		srv := httptest.NewServer(api.NewHandler(seededRepo(t), &fixedLookup{wire: zippo.Wire{Places: nil}}, 50, nil))
		defer srv.Close()
		resp, err := http.Post(srv.URL+"/contacts/normalize", "application/json", strings.NewReader(`{"contacts":[{"contact_id":"x","mailing_postal_code":"63131","mailing_country":"us"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var v struct {
			Results []struct {
				Status string          `json:"status"`
				Place  json.RawMessage `json:"place"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &v); err != nil || resp.StatusCode != http.StatusOK || len(v.Results) != 1 || v.Results[0].Status != "ok" {
			t.Fatalf("got %d %s", resp.StatusCode, body)
		}
		testutil.AssertJSONEqual(t, []byte(`{"city":null,"state":null,"state_abbreviation":null,"latitude":null,"longitude":null}`), v.Results[0].Place)
	})
	t.Run("other upstream error → 500", func(t *testing.T) {
		h.stub.fail = errors.New("HTTP GET on resource 'x' failed: 418 I'm a teapot")
		defer func() { h.stub.fail = nil }()
		if status, body := h.post(t, testutil.ReadFixture(t, "api/normalize-request.json")); status != http.StatusInternalServerError || errorOf(t, body) != h.stub.fail.Error() {
			t.Errorf("got %d %s", status, body)
		}
	})
	t.Run("a coordinate that is not a number → 400 (EXPRESSION)", func(t *testing.T) {
		bad := "north"
		lookup := &fixedLookup{wire: zippo.Wire{Places: []zippo.PlaceWire{{Latitude: &bad}}}}
		srv := httptest.NewServer(api.NewHandler(seededRepo(t), lookup, 50, nil))
		defer srv.Close()
		resp, err := http.Post(srv.URL+"/contacts/normalize", "application/json", strings.NewReader(`{"contacts":[{"mailing_postal_code":"1","mailing_country":"us"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusBadRequest || errorOf(t, body) != "Cannot coerce String (north) to Number" {
			t.Errorf("got %d %s", resp.StatusCode, body)
		}
	})
	t.Run("max contacts is configurable", func(t *testing.T) {
		srv := httptest.NewServer(api.NewHandler(seededRepo(t), h.stub, 2, nil))
		defer srv.Close()
		resp, err := http.Post(srv.URL+"/contacts/normalize", "application/json", bytes.NewReader(testutil.ReadFixture(t, "api/normalize-request.json")))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusBadRequest || errorOf(t, body) != "At most 2 contacts per request" {
			t.Errorf("got %d %s", resp.StatusCode, body)
		}
	})
}

type fixedLookup struct{ wire zippo.Wire }

func (f *fixedLookup) Lookup(context.Context, string, string) (zippo.Wire, error) { return f.wire, nil }

// The transforms of the flows, called directly.
func TestTransforms(t *testing.T) {
	s := func(v string) *string { return &v }
	var wire zippo.Wire
	testutil.ReadJSON(t, "api/zippopotam-us-63131.json", &wire)
	contact := api.NormalizeContact{ContactID: s("003Hu0000000001IAA"), MailingPostalCode: s("63131"), MailingCountry: s("us")}

	var want struct {
		Results []json.RawMessage `json:"results"`
	}
	testutil.ReadJSON(t, "api/normalize-response.json", &want)

	ok, err := api.ZippoToResult(contact, wire)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(ok)
	testutil.AssertJSONEqual(t, want.Results[0], got)

	notFound := api.NotFoundResult(api.NormalizeContact{ContactID: s("003Hu0000000099IAA"), MailingPostalCode: s("00000"), MailingCountry: s("US")})
	got, _ = json.Marshal(notFound)
	testutil.AssertJSONEqual(t, want.Results[2], got)

	if resp := api.ResultsToJSON(nil); resp.Count != 0 || resp.Results == nil {
		t.Errorf("ResultsToJSON(nil) = %+v, want an empty array", resp)
	}
	if v := api.RowsToJSON("x", nil); v.Count != 0 || v.Contacts == nil || v.City != "x" {
		t.Errorf("RowsToJSON(nil) = %+v", v)
	}
	if err := api.ValidateNormalize([]api.NormalizeContact{{MailingPostalCode: s("1"), MailingCountry: s("us")}}, 1); err != nil {
		t.Errorf("valid input rejected: %v", err)
	}
	err = api.ValidateNormalize([]api.NormalizeContact{{}}, 1)
	if !errors.Is(err, api.ErrPostalCodeRequired) || !errors.Is(err, api.ErrCountryRequired) {
		t.Errorf("validation:all must report both: %v", err)
	}
}

// Every required property of testdata/openapi.yaml is present in the responses.
func TestOpenAPIRequiredProperties(t *testing.T) {
	required := requiredProperties(t, testutil.Fixture(t, "openapi.yaml"))
	schema := func(name string) []string {
		list, ok := required[name]
		if !ok {
			t.Fatalf("openapi.yaml has no required list at %s; known: %v", name, keys(required))
		}
		return list
	}
	contactsByCity := schema("components.schemas.ContactsByCity")
	contactView := schema("components.schemas.ContactView")
	mailing := schema("components.schemas.ContactView.properties.mailing")
	normalizeResponse := schema("components.schemas.NormalizeResponse")
	result := schema("components.schemas.NormalizeResponse.properties.results.items")
	place := schema("components.schemas.NormalizeResponse.properties.results.items.properties.place")
	errorBody := schema("components.responses.Error.content.application/json.schema")

	h := newHost(t, seededRepo(t))

	_, body := h.get(t, "?city=Saint%20Louis")
	var byCity map[string]any
	if err := json.Unmarshal(body, &byCity); err != nil {
		t.Fatal(err)
	}
	requireKeys(t, "ContactsByCity", byCity, contactsByCity)
	items, _ := byCity["contacts"].([]any)
	if len(items) == 0 {
		t.Fatal("no contacts to check")
	}
	for _, item := range items {
		contact := item.(map[string]any)
		requireKeys(t, "ContactView", contact, contactView)
		requireKeys(t, "ContactView.mailing", contact["mailing"].(map[string]any), mailing)
	}

	_, body = h.post(t, testutil.ReadFixture(t, "api/normalize-request.json"))
	var normalized map[string]any
	if err := json.Unmarshal(body, &normalized); err != nil {
		t.Fatal(err)
	}
	requireKeys(t, "NormalizeResponse", normalized, normalizeResponse)
	results, _ := normalized["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("got %d results", len(results))
	}
	var withPlace int
	for _, r := range results {
		res := r.(map[string]any)
		requireKeys(t, "NormalizeResponse.results[]", res, result)
		if p, ok := res["place"].(map[string]any); ok {
			withPlace++
			requireKeys(t, "place", p, place)
		}
	}
	if withPlace != 2 {
		t.Errorf("%d results carry a place, want 2", withPlace)
	}

	for _, errBody := range [][]byte{
		func() []byte { _, b := h.get(t, ""); return b }(),
		func() []byte { _, b := h.post(t, []byte(`{}`)); return b }(),
	} {
		var e map[string]any
		if err := json.Unmarshal(errBody, &e); err != nil {
			t.Fatal(err)
		}
		requireKeys(t, "Error", e, errorBody)
	}
}

func requireKeys(t *testing.T, what string, obj map[string]any, required []string) {
	t.Helper()
	if len(required) == 0 {
		t.Fatalf("%s: empty required list", what)
	}
	for _, k := range required {
		if _, ok := obj[k]; !ok {
			t.Errorf("%s: required property %q missing in %v", what, k, obj)
		}
	}
}

func keys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// requiredProperties extracts every `required: [...]` list of the OpenAPI document, keyed by the
// dotted path of the enclosing YAML keys, e.g. "components.schemas.ContactView.properties.mailing".
// It reads the indentation-based subset of YAML the contract uses; no YAML library is needed.
func requiredProperties(t *testing.T, path string) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	type level struct {
		indent int
		key    string
	}
	var stack []level
	out := map[string][]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		content := strings.TrimSpace(line)
		if strings.HasPrefix(content, "- ") {
			content = strings.TrimSpace(content[2:])
			indent += 2
		}
		key, rest, found := strings.Cut(content, ":")
		if !found {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"`)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if key == "required" {
			list := strings.Trim(strings.TrimSpace(rest), "[]")
			var props []string
			for _, p := range strings.Split(list, ",") {
				if p = strings.TrimSpace(p); p != "" {
					props = append(props, p)
				}
			}
			names := make([]string, len(stack))
			for i, l := range stack {
				names[i] = l.key
			}
			out[strings.Join(names, ".")] = props
			continue
		}
		stack = append(stack, level{indent, key})
	}
	return out
}
