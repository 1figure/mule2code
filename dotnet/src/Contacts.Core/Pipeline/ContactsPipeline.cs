using System.Text.Encodings.Web;
using System.Text.Json;
using Contacts.Core.Batch;
using Contacts.Core.Csv;
using Contacts.Core.Data;
using Contacts.Core.Mapping;
using Contacts.Core.Report;
using Contacts.Core.Settings;
using Contacts.Core.Validation;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Contacts.Core.Pipeline;

/// <summary>
/// <c>contacts-batch-process-flow</c> and the flows it calls: watches the inbox, runs <c>file-processing-job</c>
/// on each CSV file, writes the errors file, sends the report, moves the file to <c>processed/</c> or <c>failed/</c>.
/// The SFTP directories became local ones (MAPPING.md deviation 1) and the job runs inline (deviation 3).
/// </summary>
public sealed class ContactsPipeline
{
    // DataWeave's JSON writer emits ', <, > and non-ASCII characters literally; .NET's default encoder would escape them.
    private static readonly JsonSerializerOptions ErrorsFileJson = new()
    {
        WriteIndented = true,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    private readonly AppSettings settings;
    private readonly IContactRepository repository;
    private readonly IReportSink reportSink;
    private readonly ILogger logger;
    private readonly TimeProvider clock;

    /// <summary>Creates the pipeline over an opened repository and a report sink.</summary>
    /// <param name="settings">Directories and batch sizes.</param>
    /// <param name="repository">Target of the bulk inserts.</param>
    /// <param name="reportSink">Where the on-complete report goes.</param>
    /// <param name="logger">Receives the TRACE loggers of the flow as <c>Debug</c>.</param>
    /// <param name="timeProvider">Source of <c>now()</c>; <see cref="TimeProvider.System"/> by default.</param>
    public ContactsPipeline(
        AppSettings settings,
        IContactRepository repository,
        IReportSink reportSink,
        ILogger<ContactsPipeline>? logger = null,
        TimeProvider? timeProvider = null)
    {
        ArgumentNullException.ThrowIfNull(settings);
        ArgumentNullException.ThrowIfNull(repository);
        ArgumentNullException.ThrowIfNull(reportSink);
        this.settings = settings;
        this.repository = repository;
        this.reportSink = reportSink;
        this.logger = logger ?? NullLogger<ContactsPipeline>.Instance;
        clock = timeProvider ?? TimeProvider.System;
    }

    /// <summary>
    /// <c>&lt;sftp:listener doc:name="On New or Updated File"&gt;</c> with its fixed-frequency scheduler: polls
    /// <c>inbox/</c> every <see cref="AppSettings.PollSeconds"/> until cancelled. A file that fails is moved to
    /// <c>failed/</c> and the watcher keeps going.
    /// </summary>
    public async Task WatchAsync(CancellationToken cancellationToken)
    {
        using var timer = new PeriodicTimer(TimeSpan.FromSeconds(Math.Max(1, settings.PollSeconds)), clock);
        do
        {
            await ProcessInboxAsync(continueOnError: true, cancellationToken).ConfigureAwait(false);
        }
        while (await timer.WaitForNextTickAsync(cancellationToken).ConfigureAwait(false));
    }

    /// <summary>
    /// One pass of the listener: processes every <c>*.csv</c> in <c>inbox/</c> in name order and returns the
    /// statistics per file. Other files are ignored.
    /// </summary>
    /// <param name="continueOnError">Whether a failing file stops the pass (<c>false</c>) or is logged and skipped.</param>
    /// <param name="cancellationToken">Cancels between files and inside I/O.</param>
    /// <exception cref="PipelineException">A file failed and <paramref name="continueOnError"/> is <c>false</c>.</exception>
    public async Task<IReadOnlyList<BatchStatistics>> ProcessInboxAsync(bool continueOnError, CancellationToken cancellationToken)
    {
        Directory.CreateDirectory(settings.InboxDir);
        var results = new List<BatchStatistics>();
        var files = Directory.EnumerateFiles(settings.InboxDir)
            .Where(f => string.Equals(Path.GetExtension(f), ".csv", StringComparison.OrdinalIgnoreCase))
            .OrderBy(f => f, StringComparer.Ordinal)
            .ToList();
        foreach (var file in files)
        {
            cancellationToken.ThrowIfCancellationRequested();
            try
            {
                results.Add(await ProcessFileAsync(file, cancellationToken).ConfigureAwait(false));
            }
            catch (PipelineException) when (continueOnError)
            {
                // Already logged and moved to failed/ by ProcessFileAsync.
            }
        }

        return results;
    }

    /// <summary>
    /// <c>contacts-batch-process-flow</c> for one file: initialization, CSV to records, the batch job, then the file
    /// is moved to <c>processed/&lt;newFilename&gt;</c>. On an error in the flow itself (reading the input, the
    /// move) the file goes to <c>failed/</c> instead and a <see cref="PipelineException"/> carries the cause. A
    /// failure inside the job's on-complete phase (the report) is only recorded in the statistics, as in Mule.
    /// </summary>
    /// <exception cref="PipelineException">Processing failed; the original error is the inner exception.</exception>
    public async Task<BatchStatistics> ProcessFileAsync(string path, CancellationToken cancellationToken)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        logger.LogDebug("Flow starting");
        logger.LogDebug("Batch configurations - Batch block size: {BlockSize} - Main aggregator size: {AggregatorSize}", settings.BatchBlockSize, settings.BatchAggregatorSize);

        // <flow-ref name="initialization-flow">
        var vars = RunVars.Initialization(Path.GetFileName(path), clock.GetLocalNow());
        try
        {
            Directory.CreateDirectory(settings.ProcessedDir);
            BatchStatistics statistics;
            logger.LogDebug("Transforming input data to Java");
            using (var reader = new StreamReader(path))
            {
                // <ee:transform doc:name="CSV to Java">
                var rows = CsvRows.Read(reader);
                logger.LogDebug("Staging Batch Job");
                statistics = await BuildJob(vars).RunAsync(rows, cancellationToken).ConfigureAwait(false);
            }

            logger.LogDebug("Batch job successfully started");

            // <sftp:listener autoDelete="true" moveToDirectory=processed renameTo=#[vars.newFilename]>, after the job
            // instead of concurrently with it (deviation 3).
            File.Move(path, Path.Combine(settings.ProcessedDir, vars.NewFilename), overwrite: true);
            logger.LogDebug("Flow ending");
            return statistics;
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            throw;
        }
        catch (Exception e)
        {
            // <error-handler><on-error-propagate type="ANY">
            logger.LogError(e, "Failed processing file {CurrentFilename}", vars.CurrentFilename);
            MoveToFailed(path, vars.CurrentFilename);
            throw new PipelineException($"Failed processing file {vars.CurrentFilename}: {e.Message}", e);
        }
    }

    // <batch:job jobName="file-processing-job" maxFailedRecords="-1" blockSize="${batch.job.block_size}">
    private BatchJob<Dictionary<string, string>> BuildJob(RunVars vars)
    {
        var job = new BatchJob<Dictionary<string, string>>("file-processing-job", logger)
        {
            MaxFailedRecords = -1,
            BlockSize = settings.BatchBlockSize,
            // <batch:on-complete>
            OnComplete = (statistics, ct) =>
            {
                logger.LogDebug("Batch job statistics: {Statistics}", JsonSerializer.Serialize(statistics));
                return SendReportAsync(vars, statistics, ct);
            },
        };

        // <batch:step name="main-processing-step">
        var mainStep = new BatchStep<Dictionary<string, string>>("main-processing-step")
        {
            Accept = AcceptPolicy.NoFailures,
            AggregatorSize = settings.BatchAggregatorSize,
            // <batch:aggregator doc:name="main-records-aggregator" size="${batch.aggregator.main.size}">
            Aggregator = async (block, ct) =>
            {
                // <ee:transform doc:name="CSV to SQL">, then <db:bulk-insert doc:name="Contact Data">
                var contacts = block.Select(r => ContactMapper.FromRow(r.Payload, settings.TimestampPrecision)).ToList();
                await repository.BulkInsertAsync(contacts, ct).ConfigureAwait(false);
            },
        };

        // <validation:is-email doc:name="Is email" email="#[payload.email]" message="Missing or invalid email">
        mainStep.Processors.Add((record, _) =>
        {
            EmailValidation.ValidateEmail(record.Payload);
            return Task.CompletedTask;
        });

        // <batch:step name="failed-records-processing-step" acceptPolicy="ONLY_FAILURES">
        var failedStep = new BatchStep<Dictionary<string, string>>("failed-records-processing-step")
        {
            Accept = AcceptPolicy.OnlyFailures,
            AggregatorSize = 0,
            // <batch:aggregator doc:name="failed-records-aggregator" streaming="true">
            Aggregator = async (block, ct) =>
            {
                logger.LogDebug("Writing errors/failed records to file");
                // <ee:transform doc:name="Create Error Record"> per record (a processor in the XML; folded into the
                // aggregator here so the step's payload type stays the CSV row), then
                // <sftp:write doc:name="Error Records" path='#[p("sftp.processed_dir") ++ vars.errorsFilename]'> as JSON.
                var errors = block.Select(ErrorRecord.Create).ToList();
                var errorsPath = Path.Combine(settings.ProcessedDir, vars.ErrorsFilename);
                await using var stream = File.Create(errorsPath);
                await JsonSerializer.SerializeAsync(stream, errors, ErrorsFileJson, ct).ConfigureAwait(false);
            },
        };

        job.Steps.Add(mainStep);
        job.Steps.Add(failedStep);
        return job;
    }

    // <flow name="send-email-report-flow">
    private async Task SendReportAsync(RunVars vars, BatchStatistics statistics, CancellationToken cancellationToken)
    {
        logger.LogDebug("Sending contacts batch report email");
        var now = clock.GetLocalNow();
        // <ee:transform doc:name="Extract Key Statistics">, <parse-template doc:name="Create Email Body">
        var items = BatchReport.KeyStatistics(vars.CurrentFilename, now - vars.StartTime, statistics);
        var html = BatchReport.Render(items, now);
        // <email:send doc:name="Contacts Batch Report Email">
        await reportSink.SendAsync(BatchReport.Subject, html, cancellationToken).ConfigureAwait(false);
        logger.LogDebug("Contacts batch report email sent successfully");
    }

    // <sftp:move doc:name="File to Failed Directory" sourcePath='#[p("sftp.new_dir") ++ vars.currentFilename]' targetPath='#[p("sftp.failed_dir")]'>
    private void MoveToFailed(string path, string filename)
    {
        try
        {
            if (File.Exists(path))
            {
                Directory.CreateDirectory(settings.FailedDir);
                File.Move(path, Path.Combine(settings.FailedDir, filename), overwrite: true);
            }
        }
        catch (IOException e)
        {
            logger.LogError(e, "Could not move {CurrentFilename} to {FailedDir}", filename, settings.FailedDir);
        }
        catch (UnauthorizedAccessException e)
        {
            logger.LogError(e, "Could not move {CurrentFilename} to {FailedDir}", filename, settings.FailedDir);
        }
    }
}

/// <summary>Processing of one input file failed; the file was moved to <c>failed/</c> and the cause is the inner exception.</summary>
public sealed class PipelineException : Exception
{
    /// <summary>Creates the exception wrapping the cause.</summary>
    public PipelineException(string message, Exception inner) : base(message, inner)
    {
    }
}
