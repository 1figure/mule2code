using System.Globalization;
using System.Text;
using System.Text.RegularExpressions;
using Contacts.Core.Batch;

namespace Contacts.Core.Report;

/// <summary>One label/value row of the report table.</summary>
/// <param name="Label">The label cell, e.g. <c>"Records read:"</c>.</param>
/// <param name="Value">The formatted value cell.</param>
public sealed record ReportItem(string Label, string Value);

/// <summary>The two DataWeave transforms of <c>send-email-report-flow</c> and the upstream HTML template.</summary>
public static partial class BatchReport
{
    /// <summary>The <c>subject</c> attribute of <c>&lt;email:send&gt;</c>.</summary>
    public const string Subject = "Mule Contacts Batch Job Report";

    private static readonly Lazy<string> UpstreamTemplate = new(() => File.ReadAllText(Resources.ReportTemplate));

    [GeneratedRegex(@"#\[(.*?)\]", RegexOptions.Singleline)]
    private static partial Regex TemplateExpression();

    /// <summary>
    /// <c>&lt;ee:transform doc:name="Extract Key Statistics"&gt;</c>: the five rows of the report.
    /// </summary>
    /// <param name="filename">The processed file (<c>vars.currentFilename</c>).</param>
    /// <param name="elapsed">Now minus <c>vars.startTime</c>.</param>
    /// <param name="statistics">The on-complete payload.</param>
    public static IReadOnlyList<ReportItem> KeyStatistics(string filename, TimeSpan elapsed, BatchStatistics statistics)
    {
        ArgumentNullException.ThrowIfNull(statistics);
        return
        [
            new ReportItem("Processed file:", filename),
            new ReportItem("Processing time:", FormatMmSs(elapsed)),
            new ReportItem("Records read:", GroupThousands(statistics.TotalRecords)),
            new ReportItem("Successful records:", GroupThousands(statistics.SuccessfulRecords)),
            new ReportItem("Failed records:", GroupThousands(statistics.FailedRecords)),
        ];
    }

    /// <summary>
    /// <c>(millis as DateTime {unit: "milliseconds"}) as String {format: "mm:ss.SSS"}</c>: minutes and seconds of the
    /// elapsed time; hours wrap into minutes as the DataWeave format did (it printed the minute-of-hour).
    /// </summary>
    public static string FormatMmSs(TimeSpan elapsed) =>
        string.Format(CultureInfo.InvariantCulture, "{0:00}:{1:00}.{2:000}", elapsed.Minutes, elapsed.Seconds, elapsed.Milliseconds);

    /// <summary><c>n as String {format: "#,###"}</c>: thousands separated by commas, <c>0</c> printed as <c>0</c>.</summary>
    public static string GroupThousands(long value) => value.ToString("#,##0", CultureInfo.InvariantCulture);

    /// <summary>
    /// <c>&lt;parse-template doc:name="Create Email Body" location="parse-template/contacts-batch-report-email.template"&gt;</c>:
    /// the upstream HTML with its two embedded expressions evaluated — the <c>now()</c> timestamp and the table rows.
    /// </summary>
    /// <param name="items">The rows from <see cref="KeyStatistics"/>.</param>
    /// <param name="now">The time the report is created.</param>
    /// <param name="template">The template text; defaults to the upstream file (read once).</param>
    public static string Render(IReadOnlyList<ReportItem> items, DateTimeOffset now, string? template = null)
    {
        ArgumentNullException.ThrowIfNull(items);
        template ??= UpstreamTemplate.Value;
        return TemplateExpression().Replace(template, match =>
        {
            var expression = match.Groups[1].Value.TrimStart();
            return expression.StartsWith("now()", StringComparison.Ordinal)
                ? FormatReportTimestamp(now)
                : RenderRows(items);
        });
    }

    /// <summary><c>now() as String {format: "eeee, MMMM d, u' at 'HH:mm:ss.SSS"}</c>, e.g. <c>Friday, September 25, 2026 at 14:03:07.123</c>.</summary>
    public static string FormatReportTimestamp(DateTimeOffset now) =>
        now.ToString("dddd, MMMM d, yyyy' at 'HH:mm:ss.fff", CultureInfo.InvariantCulture);

    // The inner DataWeave of the template: one <tr> per item, joined without separator.
    private static string RenderRows(IReadOnlyList<ReportItem> items)
    {
        var sb = new StringBuilder();
        foreach (var item in items)
        {
            sb.Append("<tr><td>").Append(item.Label).Append("</td><td>").Append(item.Value).Append("</td></tr>");
        }

        return sb.ToString();
    }
}
