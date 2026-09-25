namespace Contacts.Core.Batch;

/// <summary>
/// One record travelling through a <see cref="BatchJob{TRecord}"/>. Like a Mule batch record it keeps its payload
/// and, once it has failed, the first exception that failed it (<c>Batch::getFirstException()</c>).
/// </summary>
/// <typeparam name="TRecord">The payload type.</typeparam>
public sealed class BatchRecord<TRecord>
{
    /// <summary>Creates a record wrapping <paramref name="payload"/>, at 0-based position <paramref name="index"/> of the input.</summary>
    public BatchRecord(TRecord payload, long index)
    {
        Payload = payload;
        Index = index;
    }

    /// <summary>The record payload; a step's processor may replace it (a Mule <c>set-payload</c> inside a step).</summary>
    public TRecord Payload { get; set; }

    /// <summary>0-based position in the input.</summary>
    public long Index { get; }

    /// <summary>The first exception that failed the record, or <c>null</c> while it is successful.</summary>
    public Exception? Error { get; private set; }

    /// <summary>Whether any step has failed this record.</summary>
    public bool Failed => Error != null;

    /// <summary>Marks the record failed; only the first exception is kept.</summary>
    public void Fail(Exception error)
    {
        ArgumentNullException.ThrowIfNull(error);
        Error ??= error;
    }
}
