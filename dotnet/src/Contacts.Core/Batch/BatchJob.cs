using System.Diagnostics;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Contacts.Core.Batch;

/// <summary>
/// A small generic reproduction of the Mule <c>&lt;batch:job&gt;</c>:
/// <list type="bullet">
/// <item>the input is loaded in blocks of <see cref="BlockSize"/> records;</item>
/// <item>steps run in order, and a step finishes for every record before the next step starts;</item>
/// <item>a step accepts records by its <see cref="AcceptPolicy"/>, runs its processors per record, then hands the
/// records that passed to its aggregator in blocks of <see cref="BatchStep{TRecord}.AggregatorSize"/> (0 = all);</item>
/// <item>a throwing processor fails the record, a throwing aggregator fails every record of its block;</item>
/// <item>the job stops once more than <see cref="MaxFailedRecords"/> records have failed (-1 = unlimited);</item>
/// <item><see cref="OnComplete"/> receives the <see cref="BatchStatistics"/>.</item>
/// </list>
/// Unlike Mule the job runs synchronously on the caller (MAPPING.md deviation 3).
/// </summary>
/// <typeparam name="TRecord">The record payload type.</typeparam>
public sealed class BatchJob<TRecord>
{
    /// <summary>Creates a job with the given <c>jobName</c> attribute.</summary>
    public BatchJob(string jobName, ILogger? logger = null)
    {
        ArgumentException.ThrowIfNullOrEmpty(jobName);
        JobName = jobName;
        Logger = logger ?? NullLogger.Instance;
    }

    /// <summary>The <c>jobName</c> attribute.</summary>
    public string JobName { get; }

    /// <summary>The <c>maxFailedRecords</c> attribute; <c>-1</c> means unlimited.</summary>
    public int MaxFailedRecords { get; init; } = -1;

    /// <summary>The <c>blockSize</c> attribute: how many records are pulled from the input at a time.</summary>
    public int BlockSize { get; init; } = 100;

    /// <summary>The <c>&lt;batch:process-records&gt;</c> steps, in order.</summary>
    public IList<BatchStep<TRecord>> Steps { get; } = [];

    /// <summary>The <c>&lt;batch:on-complete&gt;</c> body; receives the statistics.</summary>
    public Func<BatchStatistics, CancellationToken, Task>? OnComplete { get; init; }

    private ILogger Logger { get; }

    /// <summary>
    /// Runs the job over <paramref name="input"/> and returns the statistics (the same object <see cref="OnComplete"/> got).
    /// </summary>
    /// <exception cref="BatchInputException">Enumerating <paramref name="input"/> threw; the job did not process any record.</exception>
    /// <exception cref="OperationCanceledException"><paramref name="cancellationToken"/> was cancelled.</exception>
    /// <exception cref="ArgumentOutOfRangeException"><see cref="BlockSize"/> is not positive.</exception>
    public async Task<BatchStatistics> RunAsync(IEnumerable<TRecord> input, CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(input);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(BlockSize);
        var instanceId = Guid.NewGuid().ToString();
        var clock = Stopwatch.StartNew();
        Logger.LogDebug("Batch job {JobName} instance {InstanceId} starting", JobName, instanceId);

        // Input / loading phase. Mule enqueues records in blocks of blockSize; the blocks are then processed step by
        // step, but every step must see all records before the next starts, so the whole input is materialised here.
        List<BatchRecord<TRecord>> records;
        try
        {
            records = Load(input, cancellationToken);
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch (Exception e)
        {
            Logger.LogError(e, "Batch job {JobName}: input phase failed", JobName);
            throw new BatchInputException($"Batch job '{JobName}' failed reading its input: {e.Message}", e);
        }

        var run = new Run(MaxFailedRecords);
        foreach (var step in Steps)
        {
            cancellationToken.ThrowIfCancellationRequested();
            Logger.LogDebug("Batch step {StepName} starting", step.Name);
            var passed = new List<BatchRecord<TRecord>>();
            foreach (var record in records)
            {
                if (!step.Accepts(record))
                {
                    continue;
                }

                if (await RunProcessorsAsync(step, record, run, cancellationToken).ConfigureAwait(false))
                {
                    passed.Add(record);
                }
                else if (run.ExceedsMaxFailures)
                {
                    break;
                }
            }

            if (!run.ExceedsMaxFailures && step.Aggregator != null)
            {
                await RunAggregatorAsync(step, passed, run, cancellationToken).ConfigureAwait(false);
            }

            if (run.ExceedsMaxFailures)
            {
                Logger.LogWarning("Batch job {JobName} stopped: more than {MaxFailedRecords} failed records", JobName, MaxFailedRecords);
                break;
            }
        }

        var statistics = new BatchStatistics
        {
            BatchJobInstanceId = instanceId,
            TotalRecords = records.Count,
            LoadedRecords = records.Count,
            ProcessedRecords = records.Count,
            SuccessfulRecords = records.Count - run.FailedCount,
            FailedRecords = run.FailedCount,
            ElapsedTimeInMillis = clock.ElapsedMilliseconds,
            FailedOnInputPhase = false,
        };

        // Like Mule, a failure in the on-complete phase does not fail the job or reach the calling flow's error
        // handler; it is recorded on the statistics (failedOnCompletePhase) and logged.
        if (OnComplete != null)
        {
            try
            {
                await OnComplete(statistics, cancellationToken).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
            {
                throw;
            }
            catch (Exception e)
            {
                Logger.LogError(e, "Batch job {JobName}: on-complete phase failed", JobName);
                statistics = statistics with { FailedOnCompletePhase = true };
            }
        }

        Logger.LogDebug("Batch job {JobName} completed: {Successful} successful, {Failed} failed", JobName, statistics.SuccessfulRecords, statistics.FailedRecords);
        return statistics;
    }

    private List<BatchRecord<TRecord>> Load(IEnumerable<TRecord> input, CancellationToken cancellationToken)
    {
        var records = new List<BatchRecord<TRecord>>();
        var index = 0L;
        foreach (var payload in input)
        {
            if (index % BlockSize == 0)
            {
                cancellationToken.ThrowIfCancellationRequested();
                Logger.LogDebug("Batch job {JobName}: loading block starting at record {Index}", JobName, index);
            }

            records.Add(new BatchRecord<TRecord>(payload, index++));
        }

        return records;
    }

    // Runs the step's processors; returns false when one of them failed the record.
    private async Task<bool> RunProcessorsAsync(BatchStep<TRecord> step, BatchRecord<TRecord> record, Run run, CancellationToken cancellationToken)
    {
        foreach (var processor in step.Processors)
        {
            try
            {
                await processor(record, cancellationToken).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
            {
                throw;
            }
            catch (Exception e)
            {
                Logger.LogDebug(e, "Batch step {StepName}: record {Index} failed", step.Name, record.Index);
                run.Fail(record, e);
                return false;
            }
        }

        return true;
    }

    // Feeds the aggregator block by block; stops early when maxFailedRecords is exceeded.
    private async Task RunAggregatorAsync(BatchStep<TRecord> step, List<BatchRecord<TRecord>> passed, Run run, CancellationToken cancellationToken)
    {
        // size="0" / streaming="true": the whole step's output in a single call.
        var size = step.AggregatorSize <= 0 ? Math.Max(passed.Count, 1) : step.AggregatorSize;
        for (var offset = 0; offset < passed.Count; offset += size)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var block = passed.GetRange(offset, Math.Min(size, passed.Count - offset));
            try
            {
                await step.Aggregator!(block, cancellationToken).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
            {
                throw;
            }
            catch (Exception e)
            {
                Logger.LogWarning(e, "Batch step {StepName}: aggregator failed a block of {Count} records", step.Name, block.Count);
                foreach (var record in block)
                {
                    run.Fail(record, e);
                }

                if (run.ExceedsMaxFailures)
                {
                    return;
                }
            }
        }
    }

    // Mutable state of one RunAsync call: the failure counter and the maxFailedRecords check.
    private sealed class Run(int maxFailedRecords)
    {
        public long FailedCount { get; private set; }

        public bool ExceedsMaxFailures => maxFailedRecords >= 0 && FailedCount > maxFailedRecords;

        public void Fail(BatchRecord<TRecord> record, Exception error)
        {
            if (!record.Failed)
            {
                FailedCount++;
            }

            record.Fail(error);
        }
    }
}

/// <summary>The input of a <see cref="BatchJob{TRecord}"/> could not be read; the original error is the inner exception.</summary>
public sealed class BatchInputException : Exception
{
    /// <summary>Creates the exception wrapping the input failure.</summary>
    public BatchInputException(string message, Exception inner) : base(message, inner)
    {
    }
}
