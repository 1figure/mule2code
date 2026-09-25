using System.Globalization;
using Contacts.Core.Model;

namespace Contacts.Core.Mapping;

/// <summary>
/// DataWeave type coercions used by the "CSV to SQL" transform: <c>as Number</c>, <c>as Boolean</c>, <c>as Date</c>.
/// A <c>null</c> or empty cell coerces to <c>null</c>, as DataWeave does; anything else that does not parse throws
/// a <see cref="CoercionException"/> naming the field, which fails the record (and its aggregator block).
/// </summary>
public static class Coerce
{
    /// <summary>DataWeave <c>as Number</c> for an integer column.</summary>
    /// <exception cref="CoercionException">The value is not an integer.</exception>
    public static int? ToInt(string? value, string field)
    {
        if (string.IsNullOrEmpty(value))
        {
            return null;
        }

        try
        {
            return int.Parse(value, NumberStyles.Integer, CultureInfo.InvariantCulture);
        }
        catch (Exception e) when (e is FormatException or OverflowException)
        {
            throw new CoercionException(field, value, "Number", e);
        }
    }

    /// <summary>DataWeave <c>as Boolean</c>: <c>"true"</c> / <c>"false"</c>, case-insensitive.</summary>
    /// <exception cref="CoercionException">The value is neither <c>true</c> nor <c>false</c>.</exception>
    public static bool? ToBool(string? value, string field)
    {
        if (string.IsNullOrEmpty(value))
        {
            return null;
        }

        if (value.Equals("true", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.Equals("false", StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        throw new CoercionException(field, value, "Boolean");
    }

    /// <summary>
    /// DataWeave <c>as Date</c> on a date column (<c>birthdate</c>): the calendar date of an ISO-8601 date or
    /// date-time, taken as written without zone conversion; a time part is dropped.
    /// </summary>
    /// <exception cref="CoercionException">The value is neither an ISO date nor an ISO date-time.</exception>
    public static DateOnly? ToDate(string? value, string field) =>
        ToTimestamp(value, field, TimestampPrecision.Date)?.Date;

    /// <summary>
    /// <c>as Date</c> on the three timestamp columns (MAPPING.md deviation 4): under
    /// <see cref="TimestampPrecision.Date"/> exactly what Mule does (date only); under
    /// <see cref="TimestampPrecision.Instant"/> the full timestamp is kept.
    /// </summary>
    /// <exception cref="CoercionException">The value is neither an ISO date nor an ISO date-time.</exception>
    public static Timestamp? ToTimestamp(string? value, string field, TimestampPrecision precision)
    {
        if (string.IsNullOrEmpty(value))
        {
            return null;
        }

        try
        {
            return Timestamp.Parse(value, precision);
        }
        catch (FormatException e)
        {
            // The XML coerces with 'as Date' in both modes, so the message names Date, as Mule's would.
            throw new CoercionException(field, value, "Date", e);
        }
    }
}

/// <summary>
/// A DataWeave coercion (<c>as Number</c>, <c>as Boolean</c>, <c>as Date</c>) failed for one field. The message is
/// DataWeave's own wording (<c>Cannot coerce String (abc) to Number</c>) because it ends up in the errors file as
/// <c>Batch::getFirstException().message</c>; the field is kept as a property.
/// </summary>
public sealed class CoercionException : Exception
{
    /// <summary>Creates the exception for <paramref name="field"/> whose <paramref name="value"/> is not a <paramref name="targetType"/>.</summary>
    public CoercionException(string field, string value, string targetType, Exception? inner = null)
        : base($"Cannot coerce String ({value}) to {targetType}", inner)
    {
        Field = field;
        Value = value;
        TargetType = targetType;
    }

    /// <summary>The CSV column that failed.</summary>
    public string Field { get; }

    /// <summary>The raw cell value.</summary>
    public string Value { get; }

    /// <summary>The DataWeave type the value was coerced to.</summary>
    public string TargetType { get; }
}
