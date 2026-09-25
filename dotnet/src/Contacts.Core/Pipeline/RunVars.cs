using System.Globalization;

namespace Contacts.Core.Pipeline;

/// <summary>The flow variables set by <c>initialization-flow</c>, one instance per processed file.</summary>
/// <param name="StartTime"><c>vars.startTime</c>: when processing began.</param>
/// <param name="CurrentFilename"><c>vars.currentFilename</c>: the name of the input file.</param>
/// <param name="Timestamp"><c>vars.timestamp</c>: <see cref="StartTime"/> as <c>yyyyMMdd'T'HHmmssfff</c>.</param>
/// <param name="NewFilename"><c>vars.newFilename</c>: <c>&lt;stem&gt;.&lt;timestamp&gt;.&lt;ext&gt;</c>, the name in <c>processed/</c>.</param>
/// <param name="ErrorsFilename"><c>vars.errorsFilename</c>: <c>&lt;stem&gt;.&lt;timestamp&gt;.errors.json</c>.</param>
public sealed record RunVars(
    DateTimeOffset StartTime,
    string CurrentFilename,
    string Timestamp,
    string NewFilename,
    string ErrorsFilename)
{
    /// <summary>The DataWeave <c>uuuuMMdd'T'HHmmssSSS</c> pattern in .NET spelling.</summary>
    public const string TimestampFormat = "yyyyMMdd'T'HHmmssfff";

    /// <summary>
    /// <c>initialization-flow</c>: derives the variables from the file name and the start time. The name is split at
    /// dots exactly as <c>splitBy(".")</c> did: the stem is the part before the first dot, the extension the part after it.
    /// </summary>
    public static RunVars Initialization(string filename, DateTimeOffset startTime)
    {
        ArgumentException.ThrowIfNullOrEmpty(filename);
        var timestamp = startTime.ToString(TimestampFormat, CultureInfo.InvariantCulture);
        var parts = filename.Split('.');
        var stem = parts[0];
        var extension = parts.Length > 1 ? parts[1] : "";
        return new RunVars(
            startTime,
            filename,
            timestamp,
            $"{stem}.{timestamp}.{extension}",
            $"{stem}.{timestamp}.errors.json");
    }
}
