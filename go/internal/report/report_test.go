package report

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"muletocode/internal/batch"
)

// `as String {format: "mm:ss.SSS"}`
func TestFormatMMSS(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00.000"},
		{7 * time.Millisecond, "00:00.007"},
		{1500 * time.Millisecond, "00:01.500"},
		{61*time.Second + 5*time.Millisecond, "01:01.005"},
		{59*time.Minute + 59*time.Second + 999*time.Millisecond, "59:59.999"},
		{time.Hour + 2*time.Minute, "02:00.000"}, // the DataWeave format printed the minute-of-hour
		{-5 * time.Second, "00:00.000"},
	}
	for _, tc := range cases {
		if got := formatMMSS(tc.in); got != tc.want {
			t.Errorf("formatMMSS(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// `as String {format: "#,###"}`
func TestGroupThousands(t *testing.T) {
	cases := map[int64]string{0: "0", 7: "7", 98: "98", 100: "100", 999: "999", 1000: "1,000", 12345: "12,345", 1234567: "1,234,567", -1234: "-1,234"}
	for in, want := range cases {
		if got := groupThousands(in); got != want {
			t.Errorf("groupThousands(%d) = %q, want %q", in, got, want)
		}
	}
}

// <ee:transform doc:name="Extract Key Statistics">
func TestKeyStatistics(t *testing.T) {
	stats := batch.Statistics{TotalRecords: 12345, SuccessfulRecords: 12000, FailedRecords: 345}
	items := KeyStatistics("contact-data.csv", 2*time.Minute+3*time.Second+45*time.Millisecond, stats)
	want := []Item{
		{"Processed file:", "contact-data.csv"},
		{"Processing time:", "02:03.045"},
		{"Records read:", "12,345"},
		{"Successful records:", "12,000"},
		{"Failed records:", "345"},
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d", len(items), len(want))
	}
	for i := range want {
		if items[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, items[i], want[i])
		}
	}
}

// <parse-template> over the upstream template: both #[...] expressions evaluated, the rest untouched.
func TestRender_UpstreamTemplate(t *testing.T) {
	now := time.Date(2026, time.September, 25, 14, 3, 7, 123_000_000, time.UTC)
	items := KeyStatistics("x.csv", 0, batch.Statistics{TotalRecords: 100, SuccessfulRecords: 98, FailedRecords: 2})
	html, err := Render(items, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<p>Hello,</p>",
		"on Friday, September 25, 2026 at 14:03:07.123.</p>",
		"<tr><td>Processed file:</td><td>x.csv</td></tr><tr><td>Processing time:</td><td>00:00.000</td></tr><tr><td>Records read:</td><td>100</td></tr><tr><td>Successful records:</td><td>98</td></tr><tr><td>Failed records:</td><td>2</td></tr>",
		"<p>Please do not hesitate to contact the support team with any questions.</p>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered report lacks %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "#[") || strings.Contains(html, "{{") {
		t.Errorf("unevaluated expression left in:\n%s", html)
	}
}

// parse-template concatenates item.label / item.value as they are; the port must not escape them.
func TestRenderTemplate_VerbatimValues(t *testing.T) {
	html, err := RenderTemplate("<b>#[now() as String]</b> #[payload map]", []Item{{"<x>", "a&b"}}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<tr><td><x></td><td>a&b</td></tr>") {
		t.Errorf("values must be inserted verbatim, as DataWeave did: %s", html)
	}
	if _, err := RenderTemplate("{{.Broken", nil, time.Time{}); err == nil {
		t.Error("a template that does not parse must be an error")
	}
}

func TestFileSink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	sink := NewFileSink(dir)
	sink.Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC) }
	if err := sink.Send(context.Background(), Subject, "<p>x</p>"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "20260102T030405678.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "<!-- Mule Contacts Batch Job Report -->\n<p>x</p>" {
		t.Errorf("content = %q", data)
	}
	if err := NewFileSink(filepath.Join(os.DevNull, "x")).Send(context.Background(), Subject, ""); err == nil {
		t.Error("an unwritable directory must be an error")
	}
}

func TestStdoutSink(t *testing.T) {
	var sb strings.Builder
	if err := NewStdoutSink(&sb).Send(context.Background(), Subject, "<p>x</p>"); err != nil {
		t.Fatal(err)
	}
	if sb.String() != "Subject: Mule Contacts Batch Job Report\n<p>x</p>\n" {
		t.Errorf("output = %q", sb.String())
	}
	if NewStdoutSink(nil).W != os.Stdout {
		t.Error("nil writer must default to stdout")
	}
}
