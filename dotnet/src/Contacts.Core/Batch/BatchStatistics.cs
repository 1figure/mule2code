using System.Text.Json.Serialization;

namespace Contacts.Core.Batch;

/// <summary>
/// The statistics object that <c>&lt;batch:on-complete&gt;</c> receives as its payload. The field names follow the
/// Mule <c>BatchJobResult</c> (see <c>examples/batch-job-statistics-example.json</c>); the <c>*PhaseException</c>
/// fields are not reproduced (MAPPING.md deviation 8).
/// </summary>
public sealed record BatchStatistics
{
    /// <summary>Unique id of this job run.</summary>
    [JsonPropertyName("batchJobInstanceId")]
    public required string BatchJobInstanceId { get; init; }

    /// <summary>Records read from the input.</summary>
    [JsonPropertyName("totalRecords")]
    public long TotalRecords { get; init; }

    /// <summary>Records loaded into the job queue (equals <see cref="TotalRecords"/> unless the input phase failed).</summary>
    [JsonPropertyName("loadedRecords")]
    public long LoadedRecords { get; init; }

    /// <summary>Records that went through the process phase.</summary>
    [JsonPropertyName("processedRecords")]
    public long ProcessedRecords { get; init; }

    /// <summary>Records that no step failed.</summary>
    [JsonPropertyName("successfulRecords")]
    public long SuccessfulRecords { get; init; }

    /// <summary>Records that at least one step failed.</summary>
    [JsonPropertyName("failedRecords")]
    public long FailedRecords { get; init; }

    /// <summary>Wall-clock time of the job in milliseconds.</summary>
    [JsonPropertyName("elapsedTimeInMillis")]
    public long ElapsedTimeInMillis { get; init; }

    /// <summary>Whether reading the input failed before any record was processed.</summary>
    [JsonPropertyName("failedOnInputPhase")]
    public bool FailedOnInputPhase { get; init; }

    /// <summary>Whether the <c>&lt;batch:on-complete&gt;</c> body threw; the job itself still completed.</summary>
    [JsonPropertyName("failedOnCompletePhase")]
    public bool FailedOnCompletePhase { get; init; }
}
