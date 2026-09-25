namespace Contacts.Core.Batch;

/// <summary>The <c>acceptPolicy</c> of a <c>&lt;batch:step&gt;</c>.</summary>
public enum AcceptPolicy
{
    /// <summary>Only records that no previous step has failed (the default).</summary>
    NoFailures,

    /// <summary>Only records that a previous step has failed.</summary>
    OnlyFailures,

    /// <summary>Every record.</summary>
    All,
}

/// <summary>A per-record processor of a step; throwing fails the record.</summary>
/// <typeparam name="TRecord">The record payload type.</typeparam>
public delegate Task RecordProcessor<TRecord>(BatchRecord<TRecord> record, CancellationToken cancellationToken);

/// <summary>
/// The body of a <c>&lt;batch:aggregator&gt;</c>: receives one block of records that passed the step's processors.
/// Throwing fails every record of the block.
/// </summary>
/// <typeparam name="TRecord">The record payload type.</typeparam>
public delegate Task BlockAggregator<TRecord>(IReadOnlyList<BatchRecord<TRecord>> block, CancellationToken cancellationToken);

/// <summary>
/// A <c>&lt;batch:step&gt;</c>: an accept policy, a list of per-record processors and an optional aggregator.
/// </summary>
/// <typeparam name="TRecord">The record payload type.</typeparam>
public sealed class BatchStep<TRecord>
{
    /// <summary>Creates a step with the given <c>name</c> attribute.</summary>
    public BatchStep(string name)
    {
        ArgumentException.ThrowIfNullOrEmpty(name);
        Name = name;
    }

    /// <summary>The step name, for logs.</summary>
    public string Name { get; }

    /// <summary>The <c>acceptPolicy</c> attribute; default <see cref="AcceptPolicy.NoFailures"/>.</summary>
    public AcceptPolicy Accept { get; init; } = AcceptPolicy.NoFailures;

    /// <summary>Processors run for each accepted record, in order, before the aggregator.</summary>
    public IList<RecordProcessor<TRecord>> Processors { get; } = [];

    /// <summary>The aggregator body, or <c>null</c> when the step has no <c>&lt;batch:aggregator&gt;</c>.</summary>
    public BlockAggregator<TRecord>? Aggregator { get; init; }

    /// <summary>
    /// The aggregator's <c>size</c> attribute; <c>0</c> means <c>streaming="true"</c>, all records of the step in one call.
    /// </summary>
    public int AggregatorSize { get; init; }

    /// <summary>Whether a record enters this step under <see cref="Accept"/>.</summary>
    public bool Accepts(BatchRecord<TRecord> record)
    {
        ArgumentNullException.ThrowIfNull(record);
        return Accept switch
        {
            AcceptPolicy.NoFailures => !record.Failed,
            AcceptPolicy.OnlyFailures => record.Failed,
            _ => true,
        };
    }
}
