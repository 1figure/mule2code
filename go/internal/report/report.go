// Package report is send-email-report-flow: the "Extract Key Statistics" transform, the
// parse-template over the upstream HTML e-mail template, and the sink that replaces <email:send>
// (MAPPING.md deviation 2).
package report

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"muletocode/internal/batch"
	"muletocode/internal/resources"
)

// Subject is the subject attribute of <email:send>.
const Subject = "Mule Contacts Batch Job Report"

// Item is one label/value row of the report table.
type Item struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// KeyStatistics is <ee:transform doc:name="Extract Key Statistics">: the five rows of the report.
// filename is vars.currentFilename, elapsed is now minus vars.startTime, stats the on-complete payload.
func KeyStatistics(filename string, elapsed time.Duration, stats batch.Statistics) []Item {
	return []Item{
		{Label: "Processed file:", Value: filename},
		{Label: "Processing time:", Value: formatMMSS(elapsed)},
		{Label: "Records read:", Value: groupThousands(stats.TotalRecords)},
		{Label: "Successful records:", Value: groupThousands(stats.SuccessfulRecords)},
		{Label: "Failed records:", Value: groupThousands(stats.FailedRecords)},
	}
}

// formatMMSS is `(millis as DateTime {unit: "milliseconds"}) as String {format: "mm:ss.SSS"}`:
// minutes and seconds of the elapsed time; hours wrap into the minute-of-hour as the DataWeave format did.
func formatMMSS(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	minutes := int(elapsed/time.Minute) % 60
	seconds := int(elapsed/time.Second) % 60
	millis := int(elapsed/time.Millisecond) % 1000
	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, millis)
}

// groupThousands is `n as String {format: "#,###"}`: thousands separated by commas, 0 printed as 0.
func groupThousands(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var sb strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(d)
	}
	return sign + sb.String()
}

// The two #[...] expressions embedded in the upstream template.
var templateExpression = regexp.MustCompile(`(?s)#\[(.*?)\]`)

// Render is <parse-template doc:name="Create Email Body" location="parse-template/contacts-batch-report-email.template">: the
// upstream HTML with its two embedded DataWeave expressions evaluated, the now() timestamp and the
// table rows built from items.
func Render(items []Item, now time.Time) (string, error) {
	text, err := resources.ReportTemplate()
	if err != nil {
		return "", err
	}
	return RenderTemplate(string(text), items, now)
}

// RenderTemplate is Render over the given template text instead of the upstream file. Labels and
// values are inserted verbatim, as parse-template concatenated them: no HTML escaping.
func RenderTemplate(text string, items []Item, now time.Time) (string, error) {
	// Each DataWeave expression becomes the equivalent text/template action:
	// the now() format → {{.Now}}; the map/joinBy over the items → a range.
	goText := templateExpression.ReplaceAllStringFunc(text, func(match string) string {
		expression := strings.TrimSpace(match[2 : len(match)-1])
		if strings.HasPrefix(expression, "now()") {
			return "{{.Now}}"
		}
		return "{{range .Items}}<tr><td>{{.Label}}</td><td>{{.Value}}</td></tr>{{end}}"
	})
	tmpl, err := template.New("report").Parse(goText)
	if err != nil {
		return "", fmt.Errorf("parsing report template: %w", err)
	}
	var sb strings.Builder
	data := struct {
		Now   string
		Items []Item
	}{formatReportTimestamp(now), items}
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("rendering report: %w", err)
	}
	return sb.String(), nil
}

// formatReportTimestamp is `now() as String {format: "eeee, MMMM d, u' at 'HH:mm:ss.SSS"}`,
// e.g. "Friday, September 25, 2026 at 14:03:07.123".
func formatReportTimestamp(now time.Time) string {
	return now.Format("Monday, January 2, 2006 at 15:04:05.000")
}
