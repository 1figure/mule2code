using System.Globalization;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Contacts.Core.Model;

/// <summary>How the three timestamp columns are coerced (<c>TIMESTAMP_PRECISION</c>, MAPPING.md deviation 4).</summary>
public enum TimestampPrecision
{
    /// <summary>Mule's <c>as Date</c>: only the calendar date, as written, survives (default).</summary>
    Date,

    /// <summary>Keep the full instant (UTC, milliseconds); a date-only input stays a date.</summary>
    Instant,
}

/// <summary>
/// A value of <c>created_date</c>, <c>last_modified_date</c> or <c>system_mod_stamp</c>: an instant in UTC plus
/// whether it carries a time part at all. <c>2025-05-01</c> and <c>2025-05-01T11:39:15.257Z</c> both round-trip
/// unchanged through <see cref="ToString"/>, which is also the SQLite representation.
/// </summary>
[JsonConverter(typeof(TimestampJsonConverter))]
public readonly record struct Timestamp
{
    private const string DateFormat = "yyyy-MM-dd";
    private const string InstantFormat = "yyyy-MM-dd'T'HH:mm:ss.fff'Z'";

    private static readonly string[] Formats =
    [
        DateFormat,
        "yyyy-MM-dd'T'HH:mm:ss.FFFFFFFK",
        "yyyy-MM-dd'T'HH:mm:ssK",
        "yyyy-MM-dd'T'HH:mm:ss.FFFFFFF",
        "yyyy-MM-dd'T'HH:mm:ss",
    ];

    /// <summary>
    /// Creates a value. With <paramref name="isDateOnly"/> the calendar date of <paramref name="value"/> is taken as
    /// written in its own offset and stored as midnight UTC; otherwise the instant is normalised to UTC.
    /// </summary>
    public Timestamp(DateTimeOffset value, bool isDateOnly)
    {
        IsDateOnly = isDateOnly;
        Value = isDateOnly
            ? new DateTimeOffset(DateOnly.FromDateTime(value.DateTime).ToDateTime(TimeOnly.MinValue), TimeSpan.Zero)
            : value.ToUniversalTime();
    }

    /// <summary>The instant in UTC; midnight for a date-only value.</summary>
    public DateTimeOffset Value { get; }

    /// <summary>Whether the value carries no time part.</summary>
    public bool IsDateOnly { get; }

    /// <summary>The calendar date.</summary>
    public DateOnly Date => DateOnly.FromDateTime(Value.UtcDateTime);

    /// <summary>
    /// Parses an ISO-8601 date or date-time. Under <see cref="TimestampPrecision.Date"/> the result is always
    /// date-only (the date as written, no zone conversion — DataWeave's <c>as Date</c>); under
    /// <see cref="TimestampPrecision.Instant"/> a date-time keeps its time part and a plain date stays a date.
    /// A date-time without an offset is taken as UTC.
    /// </summary>
    /// <exception cref="FormatException">The text is neither an ISO date nor an ISO date-time.</exception>
    public static Timestamp Parse(string text, TimestampPrecision precision)
    {
        ArgumentNullException.ThrowIfNull(text);
        var dateOnlyInput = DateOnly.TryParseExact(text, DateFormat, CultureInfo.InvariantCulture, DateTimeStyles.None, out _);
        var parsed = DateTimeOffset.ParseExact(text, Formats, CultureInfo.InvariantCulture, DateTimeStyles.AssumeUniversal);
        return new Timestamp(parsed, isDateOnly: dateOnlyInput || precision == TimestampPrecision.Date);
    }

    /// <summary><c>yyyy-MM-dd</c> for a date-only value, otherwise UTC with milliseconds and a <c>Z</c>.</summary>
    public override string ToString() =>
        Value.ToString(IsDateOnly ? DateFormat : InstantFormat, CultureInfo.InvariantCulture);
}

/// <summary>Serialises <see cref="Timestamp"/> as its <see cref="Timestamp.ToString"/> text and reads it back as written.</summary>
public sealed class TimestampJsonConverter : JsonConverter<Timestamp>
{
    /// <inheritdoc />
    public override Timestamp Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options) =>
        Timestamp.Parse(reader.GetString() ?? throw new JsonException("Expected an ISO-8601 string"), TimestampPrecision.Instant);

    /// <inheritdoc />
    public override void Write(Utf8JsonWriter writer, Timestamp value, JsonSerializerOptions options)
    {
        ArgumentNullException.ThrowIfNull(writer);
        writer.WriteStringValue(value.ToString());
    }
}
