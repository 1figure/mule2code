using Contacts.Core.Batch;
using Contacts.Core.Report;
using Contacts.Tests.Support;

namespace Contacts.Tests.Golden;

/// <summary>"Extract Key Statistics", the two DataWeave string formats, and <c>parse-template</c>.</summary>
public sealed class ReportFormattingTests
{
    [Theory]
    [InlineData(0, "00:00.000")]
    [InlineData(379, "00:00.379")]
    [InlineData(62_345, "01:02.345")]
    [InlineData(599_999, "09:59.999")]
    [InlineData(3_600_000, "00:00.000")] // mm:ss.SSS has no hours field
    public void FormatMmSsMatchesTheDataWeavePattern(long millis, string expected) =>
        Assert.Equal(expected, BatchReport.FormatMmSs(TimeSpan.FromMilliseconds(millis)));

    [Theory]
    [InlineData(0, "0")]
    [InlineData(98, "98")]
    [InlineData(100, "100")]
    [InlineData(1_000, "1,000")]
    [InlineData(1_234_567, "1,234,567")]
    public void GroupThousandsMatchesTheDataWeavePattern(long value, string expected) =>
        Assert.Equal(expected, BatchReport.GroupThousands(value));

    [Fact]
    public void KeyStatisticsProducesTheFiveRowsFromTheGoldenReport()
    {
        var golden = TestData.ReadJson("expected/batch-report-100-with-errors.json");
        var statistics = new BatchStatistics
        {
            BatchJobInstanceId = "golden",
            TotalRecords = golden["totalRecords"]!.GetValue<long>(),
            SuccessfulRecords = golden["successfulRecords"]!.GetValue<long>(),
            FailedRecords = golden["failedRecords"]!.GetValue<long>(),
        };

        var items = BatchReport.KeyStatistics("contact-data-100-with-errors.csv", TimeSpan.FromMilliseconds(1234), statistics);

        Assert.Equal(
            [
                new ReportItem("Processed file:", "contact-data-100-with-errors.csv"),
                new ReportItem("Processing time:", "00:01.234"),
                new ReportItem("Records read:", "100"),
                new ReportItem("Successful records:", "98"),
                new ReportItem("Failed records:", "2"),
            ],
            items);
    }

    [Fact]
    public void RenderFillsTheUpstreamTemplate()
    {
        var items = new[] { new ReportItem("Records read:", "1,000"), new ReportItem("Failed records:", "0") };
        var now = new DateTimeOffset(2026, 9, 25, 14, 3, 7, 123, TimeSpan.Zero);

        var html = BatchReport.Render(items, now);

        Assert.Contains("<p>The following is the report for the Contacts batch job on Friday, September 25, 2026 at 14:03:07.123.</p>", html, StringComparison.Ordinal);
        Assert.Contains("<tr><td>Records read:</td><td>1,000</td></tr><tr><td>Failed records:</td><td>0</td></tr>", html, StringComparison.Ordinal);
        Assert.Contains("<p>Please do not hesitate to contact the support team with any questions.</p>", html, StringComparison.Ordinal);
        Assert.DoesNotContain("#[", html, StringComparison.Ordinal);
    }

    [Fact]
    public void RenderUsesTheUpstreamTemplateFileVerbatimOutsideTheExpressions()
    {
        var template = File.ReadAllText(Path.Combine(TestData.Root, "..", "mule", "batch-contacts-csv-to-db", "src", "main", "resources", "parse-template", "contacts-batch-report-email.template"));

        var html = BatchReport.Render([], DateTimeOffset.UnixEpoch, template);

        Assert.StartsWith("<p>Hello,</p>", html, StringComparison.Ordinal);
        Assert.Contains("<table>\n  <tbody>\n\n  </tbody>\n</table>", html, StringComparison.Ordinal);
    }

    [Fact]
    public async Task FileSinkWritesOneHtmlPerReport()
    {
        using var dir = new TempDir();
        var sink = new FileReportSink(dir.Sub("reports"), new FixedTimeProvider(new DateTimeOffset(2026, 1, 2, 3, 4, 5, 678, TimeSpan.Zero)));

        await sink.SendAsync(BatchReport.Subject, "<p>x</p>", CancellationToken.None);

        var file = Assert.Single(Directory.GetFiles(dir.Sub("reports")));
        Assert.Equal("20260102T030405678.html", Path.GetFileName(file));
        Assert.Equal($"<!-- {BatchReport.Subject} -->\n<p>x</p>", await File.ReadAllTextAsync(file));
    }

    [Fact]
    public async Task ConsoleSinkPrintsSubjectThenBody()
    {
        using var writer = new StringWriter();

        await new ConsoleReportSink(writer).SendAsync("S", "<p>x</p>", CancellationToken.None);

        Assert.Equal("Subject: S\n<p>x</p>\n", writer.ToString().Replace("\r\n", "\n", StringComparison.Ordinal));
    }
}
