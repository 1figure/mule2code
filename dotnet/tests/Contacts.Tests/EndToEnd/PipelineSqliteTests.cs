using System.Text.RegularExpressions;
using Contacts.Core.Csv;
using Contacts.Core.Data;
using Contacts.Core.Model;
using Contacts.Core.Pipeline;
using Contacts.Core.Report;
using Contacts.Core.Settings;
using Contacts.Tests.Support;
using Microsoft.Data.Sqlite;

namespace Contacts.Tests.EndToEnd;

/// <summary><c>contacts-batch-process-flow</c> end to end on SQLite in a temp directory.</summary>
public sealed class PipelineSqliteTests : IDisposable
{
    private static readonly DateTimeOffset FixedNow = new(2026, 1, 2, 3, 4, 5, 678, TimeSpan.Zero);
    private const string FixedTimestamp = "20260102T030405678";

    private readonly TempDir dir = new();
    private readonly AppSettings settings;
    private readonly CapturingReportSink sink = new();

    public PipelineSqliteTests()
    {
        settings = new AppSettings
        {
            DataDir = dir.Path,
            SqlitePath = Path.Combine(dir.Path, "contacts.db"),
            BatchBlockSize = 10000,
            BatchAggregatorSize = 1000,
        };
    }

    public void Dispose() => dir.Dispose();

    private async Task<ContactsPipeline> PipelineAsync(int? aggregatorSize = null, IReportSink? reportSink = null, TimestampPrecision precision = TimestampPrecision.Date)
    {
        var effective = settings with { BatchAggregatorSize = aggregatorSize ?? settings.BatchAggregatorSize, TimestampPrecision = precision };
        var repository = await SqlContactRepository.OpenAsync(DbProfile.Sqlite, effective.ConnectionString, CancellationToken.None);
        return new ContactsPipeline(effective, repository, reportSink ?? sink, timeProvider: new FixedTimeProvider(FixedNow));
    }

    private string Drop(string fixture, string? asName = null)
    {
        var target = Path.Combine(dir.Sub("inbox"), asName ?? Path.GetFileName(fixture));
        File.Copy(TestData.Fixture(fixture), target);
        return target;
    }

    private async Task<string> WriteCsvAsync(string name, IEnumerable<Dictionary<string, string>> rows)
    {
        var lines = new[] { string.Join(',', Contact.Columns) }.Concat(rows.Select(r => CsvRows.ToLine(r, Contact.Columns)));
        var file = Path.Combine(dir.Sub("inbox"), name);
        await File.WriteAllLinesAsync(file, lines);
        return file;
    }

    private async Task<long> CountAsync(string sql)
    {
        await using var connection = new SqliteConnection(settings.ConnectionString);
        await connection.OpenAsync();
        await using var command = connection.CreateCommand();
        command.CommandText = sql;
        return (long)(await command.ExecuteScalarAsync())!;
    }

    [Fact]
    public async Task CleanFileInsertsAllRowsAndReportsTheGoldenStatistics()
    {
        var pipeline = await PipelineAsync();
        var file = Drop("contact-data-100.csv");

        var stats = await pipeline.ProcessFileAsync(file, CancellationToken.None);

        Assert.Equal(100, await CountAsync("SELECT COUNT(*) FROM contacts"));
        TestData.AssertStatistics("expected/batch-report-100.json", stats);
        Assert.False(stats.FailedOnCompletePhase);
        Assert.False(File.Exists(file));
        Assert.True(File.Exists(Path.Combine(settings.ProcessedDir, $"contact-data-100.{FixedTimestamp}.csv")));
        Assert.Empty(Directory.GetFiles(settings.ProcessedDir, "*.errors.json"));
        var report = Assert.Single(sink.Reports);
        Assert.Equal("Mule Contacts Batch Job Report", report.Subject);
        Assert.Contains("<tr><td>Processed file:</td><td>contact-data-100.csv</td></tr>", report.Html, StringComparison.Ordinal);
        Assert.Contains("<tr><td>Successful records:</td><td>100</td></tr><tr><td>Failed records:</td><td>0</td></tr>", report.Html, StringComparison.Ordinal);
    }

    [Fact]
    public async Task FileWithInvalidEmailsInserts98RowsAndWritesTheGoldenErrorsFile()
    {
        var pipeline = await PipelineAsync();
        Drop("contact-data-100-with-errors.csv");

        var results = await pipeline.ProcessInboxAsync(continueOnError: false, CancellationToken.None);

        var stats = Assert.Single(results);
        Assert.Equal(98, await CountAsync("SELECT COUNT(*) FROM contacts"));
        Assert.Equal(0, await CountAsync("SELECT COUNT(*) FROM contacts WHERE external_id IN ('3', '7')"));
        TestData.AssertStatistics("expected/batch-report-100-with-errors.json", stats);

        var processed = Path.Combine(settings.ProcessedDir, $"contact-data-100-with-errors.{FixedTimestamp}.csv");
        var errors = Path.Combine(settings.ProcessedDir, $"contact-data-100-with-errors.{FixedTimestamp}.errors.json");
        Assert.True(File.Exists(processed));
        Assert.True(File.Exists(errors));
        Assert.Equal(
            Json.Canonical(TestData.ReadText("expected/errors-100-with-errors.json")),
            Json.Canonical(await File.ReadAllTextAsync(errors)));
        Assert.Contains("<tr><td>Successful records:</td><td>98</td></tr><tr><td>Failed records:</td><td>2</td></tr>", Assert.Single(sink.Reports).Html, StringComparison.Ordinal);
    }

    [Fact]
    public async Task ErrorsFileWritesApostrophesAndNonAsciiLiterallyLikeDataWeave()
    {
        var pipeline = await PipelineAsync();
        var rows = TestData.ReadRows("contact-data-100.csv").Take(3).ToList();
        rows[1]["last_name"] = "O'Brien";
        rows[1]["mailing_street"] = "12 Rue de l'Église <A&B>";
        rows[1]["email"] = "not-an-email";
        await WriteCsvAsync("quoting.csv", rows);

        await pipeline.ProcessInboxAsync(continueOnError: false, CancellationToken.None);

        var text = await File.ReadAllTextAsync(Path.Combine(settings.ProcessedDir, $"quoting.{FixedTimestamp}.errors.json"));
        Assert.Contains("O'Brien", text, StringComparison.Ordinal);
        Assert.Contains("Rue de l'Église <A&B>", text, StringComparison.Ordinal);
        Assert.DoesNotContain("\\u0027", text, StringComparison.Ordinal);
        Assert.DoesNotContain("\\u00", text, StringComparison.Ordinal);
    }

    [Fact]
    public async Task SqliteStoresValuesAsMappingDeviation7Prescribes()
    {
        var pipeline = await PipelineAsync();
        await pipeline.ProcessFileAsync(Drop("contact-data-100.csv"), CancellationToken.None);

        await using var connection = new SqliteConnection(settings.ConnectionString);
        await connection.OpenAsync();
        await using var command = connection.CreateCommand();
        command.CommandText = "SELECT typeof(may_edit), may_edit, typeof(do_not_call), do_not_call, birthdate, created_date, last_modified_date, active_tracker_count, fax FROM contacts WHERE external_id = '1'";
        await using var reader = await command.ExecuteReaderAsync();
        Assert.True(await reader.ReadAsync());

        Assert.Equal("integer", reader.GetString(0));
        Assert.Equal(1L, reader.GetInt64(1));
        Assert.Equal("integer", reader.GetString(2));
        Assert.Equal(0L, reader.GetInt64(3));
        Assert.Equal("1976-12-18", reader.GetString(4));
        Assert.Equal("2007-09-26", reader.GetString(5));
        Assert.Equal("2025-05-01", reader.GetString(6)); // 'as Date' dropped the time part (deviation 4)
        Assert.Equal(0L, reader.GetInt64(7));
        Assert.Equal("", reader.GetString(8));
    }

    [Fact]
    public async Task InstantPrecisionStoresTheFullTimestampInSqlite()
    {
        var pipeline = await PipelineAsync(precision: TimestampPrecision.Instant);
        await pipeline.ProcessFileAsync(Drop("contact-data-100.csv"), CancellationToken.None);

        await using var connection = new SqliteConnection(settings.ConnectionString);
        await connection.OpenAsync();
        await using var command = connection.CreateCommand();
        command.CommandText = "SELECT created_date, last_modified_date, system_mod_stamp, birthdate FROM contacts WHERE external_id = '1'";
        await using var reader = await command.ExecuteReaderAsync();
        Assert.True(await reader.ReadAsync());

        Assert.Equal("2007-09-26", reader.GetString(0));
        Assert.Equal("2025-05-01T11:39:15.257Z", reader.GetString(1));
        Assert.Equal("2025-05-01T11:39:15.257Z", reader.GetString(2));
        Assert.Equal("1976-12-18", reader.GetString(3));
    }

    [Fact]
    public async Task CoercionErrorFailsExactlyOneAggregatorBlock()
    {
        var pipeline = await PipelineAsync(aggregatorSize: 10);
        var rows = TestData.ReadRows("contact-data-100.csv");
        rows[14]["active_tracker_count"] = "abc"; // external_id 15 → second block of ten (11..20)
        var file = await WriteCsvAsync("coercion.csv", rows);

        var stats = await pipeline.ProcessFileAsync(file, CancellationToken.None);

        Assert.Equal(90, stats.SuccessfulRecords);
        Assert.Equal(10, stats.FailedRecords);
        Assert.Equal(90, await CountAsync("SELECT COUNT(*) FROM contacts"));
        Assert.Equal(0, await CountAsync("SELECT COUNT(*) FROM contacts WHERE CAST(external_id AS INTEGER) BETWEEN 11 AND 20"));
        var errors = TestData.ReadJson($"{settings.ProcessedDir}/coercion.{FixedTimestamp}.errors.json").AsArray();
        Assert.Equal(10, errors.Count);
        Assert.All(errors, e => Assert.Equal("Cannot coerce String (abc) to Number", e!["Error"]!.GetValue<string>()));
    }

    [Fact]
    public async Task FailingReportSinkIsRecordedOnTheStatisticsAndTheFileStillMovesToProcessed()
    {
        var pipeline = await PipelineAsync(reportSink: new FailingSink());
        var file = Drop("contact-data-100.csv");

        var stats = await pipeline.ProcessFileAsync(file, CancellationToken.None);

        Assert.True(stats.FailedOnCompletePhase);
        Assert.Equal(100, stats.SuccessfulRecords);
        Assert.Equal(100, await CountAsync("SELECT COUNT(*) FROM contacts"));
        Assert.True(File.Exists(Path.Combine(settings.ProcessedDir, $"contact-data-100.{FixedTimestamp}.csv")));
        Assert.False(Directory.Exists(settings.FailedDir));
    }

    [Fact]
    public async Task EmptyFileIsMovedToFailedWithoutAReport()
    {
        var pipeline = await PipelineAsync();
        var file = Path.Combine(dir.Sub("inbox"), "empty.csv");
        await File.WriteAllTextAsync(file, "");

        var e = await Assert.ThrowsAsync<PipelineException>(() => pipeline.ProcessFileAsync(file, CancellationToken.None));

        Assert.IsType<Core.Batch.BatchInputException>(e.InnerException);
        Assert.IsType<CsvFormatException>(e.InnerException!.InnerException);
        Assert.Contains("empty.csv", e.Message, StringComparison.Ordinal);
        Assert.False(File.Exists(file));
        Assert.True(File.Exists(Path.Combine(settings.FailedDir, "empty.csv")));
        Assert.Empty(sink.Reports);
        Assert.Empty(Directory.Exists(settings.ProcessedDir) ? Directory.GetFiles(settings.ProcessedDir) : []);
        Assert.Equal(0, await CountAsync("SELECT COUNT(*) FROM contacts"));
    }

    [Fact]
    public async Task InboxPassContinuesPastAFailedFileWhenAsked()
    {
        var pipeline = await PipelineAsync();
        await File.WriteAllTextAsync(Path.Combine(dir.Sub("inbox"), "a-broken.csv"), "");
        Drop("contact-data-100.csv", "b-good.csv");

        var results = await pipeline.ProcessInboxAsync(continueOnError: true, CancellationToken.None);

        Assert.Single(results);
        Assert.True(File.Exists(Path.Combine(settings.FailedDir, "a-broken.csv")));
        Assert.True(File.Exists(Path.Combine(settings.ProcessedDir, $"b-good.{FixedTimestamp}.csv")));
        Assert.Equal(100, await CountAsync("SELECT COUNT(*) FROM contacts"));
    }

    [Fact]
    public async Task InboxPassStopsAtAFailedFileWhenNotAsked()
    {
        var pipeline = await PipelineAsync();
        await File.WriteAllTextAsync(Path.Combine(dir.Sub("inbox"), "a-broken.csv"), "");
        var good = Drop("contact-data-100.csv", "b-good.csv");

        await Assert.ThrowsAsync<PipelineException>(() => pipeline.ProcessInboxAsync(continueOnError: false, CancellationToken.None));

        Assert.True(File.Exists(good));
    }

    [Fact]
    public async Task RenamedFileCarriesTheStartTimestamp()
    {
        var realClock = new ContactsPipeline(
            settings,
            await SqlContactRepository.OpenAsync(DbProfile.Sqlite, settings.ConnectionString, CancellationToken.None),
            sink);
        var before = DateTimeOffset.Now;
        Drop("contact-data-100.csv");

        await realClock.ProcessInboxAsync(continueOnError: false, CancellationToken.None);

        var processed = Path.GetFileName(Assert.Single(Directory.GetFiles(settings.ProcessedDir, "*.csv")));
        var match = Regex.Match(processed, @"^contact-data-100\.(\d{8}T\d{9})\.csv$");
        Assert.True(match.Success, processed);
        var stamp = DateTimeOffset.ParseExact(match.Groups[1].Value, RunVars.TimestampFormat, System.Globalization.CultureInfo.InvariantCulture, System.Globalization.DateTimeStyles.AssumeLocal);
        Assert.InRange(stamp, before.AddMilliseconds(-1), DateTimeOffset.Now.AddMilliseconds(1));
    }

    [Fact]
    public async Task NonCsvFilesAreIgnored()
    {
        var pipeline = await PipelineAsync();
        var notes = Path.Combine(dir.Sub("inbox"), "notes.txt");
        await File.WriteAllTextAsync(notes, "not a csv");
        var json = Path.Combine(dir.Sub("inbox"), "contacts.json");
        await File.WriteAllTextAsync(json, "[]");

        var results = await pipeline.ProcessInboxAsync(continueOnError: false, CancellationToken.None);

        Assert.Empty(results);
        Assert.True(File.Exists(notes));
        Assert.True(File.Exists(json));
        Assert.Empty(sink.Reports);
    }

    [Fact]
    public async Task WatchProcessesTheInboxUntilCancelled()
    {
        var pipeline = await PipelineAsync();
        Drop("contact-data-100.csv");
        using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() => pipeline.WatchAsync(cts.Token));

        Assert.Equal(100, await CountAsync("SELECT COUNT(*) FROM contacts"));
        Assert.Single(sink.Reports);
    }

    [Fact]
    public void InitializationDerivesTheVariablesLikeTheMuleFlow()
    {
        var vars = RunVars.Initialization("contact-data.csv", FixedNow);

        Assert.Equal("contact-data.csv", vars.CurrentFilename);
        Assert.Equal(FixedTimestamp, vars.Timestamp);
        Assert.Equal($"contact-data.{FixedTimestamp}.csv", vars.NewFilename);
        Assert.Equal($"contact-data.{FixedTimestamp}.errors.json", vars.ErrorsFilename);
        Assert.Equal(FixedNow, vars.StartTime);
    }

    private sealed class FailingSink : IReportSink
    {
        public Task SendAsync(string subject, string html, CancellationToken cancellationToken) =>
            throw new IOException("smtp down");
    }
}
