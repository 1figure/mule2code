using System.Text.Json.Serialization;
using Contacts.Core.Batch;
using Contacts.Core.Csv;
using Contacts.Core.Model;

namespace Contacts.Core.Pipeline;

/// <summary>One entry of the <c>&lt;stem&gt;.&lt;timestamp&gt;.errors.json</c> file.</summary>
/// <param name="Error"><c>Batch::getFirstException().message</c>.</param>
/// <param name="Record">The failed row re-serialised as a single CSV line without header.</param>
public sealed record ErrorRecord(
    [property: JsonPropertyName("Error")] string Error,
    [property: JsonPropertyName("Record")] string Record)
{
    /// <summary>
    /// <c>&lt;ee:transform doc:name="Create Error Record"&gt;</c>:
    /// <c>{Error: Batch::getFirstException().message, Record: write(payload, "application/csv", {header: false, lineSeparator: ""})}</c>.
    /// </summary>
    public static ErrorRecord Create(BatchRecord<Dictionary<string, string>> record)
    {
        ArgumentNullException.ThrowIfNull(record);
        var message = record.Error?.Message ?? "";
        return new ErrorRecord(message, CsvRows.ToLine(record.Payload, Contact.Columns));
    }
}
