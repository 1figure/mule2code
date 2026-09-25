using System.Globalization;
using System.Text.Json.Serialization;
using Contacts.Core.Api;
using Contacts.Core.Mapping;

namespace Contacts.Core.Zippo;

/// <summary>The answer of <c>GET https://api.zippopotam.us/{country}/{postalCode}</c>, as sent on the wire.</summary>
public sealed record ZippoWire
{
    /// <summary>The <c>"post code"</c> field.</summary>
    [JsonPropertyName("post code")]
    public string? PostCode { get; init; }

    /// <summary>The <c>"country"</c> field.</summary>
    [JsonPropertyName("country")]
    public string? Country { get; init; }

    /// <summary>The <c>"country abbreviation"</c> field.</summary>
    [JsonPropertyName("country abbreviation")]
    public string? CountryAbbreviation { get; init; }

    /// <summary>The <c>"places"</c> array; may be absent or <c>null</c> on the wire.</summary>
    [JsonPropertyName("places")]
    public IReadOnlyList<ZippoPlaceWire>? Places { get; init; }

    /// <summary>
    /// The <c>place</c> part of <c>&lt;ee:transform doc:name="Zippopotam to result"&gt;</c>: <c>places[0]</c> with
    /// <c>latitude as Number</c> / <c>longitude as Number</c>.
    /// </summary>
    /// <exception cref="CoercionException">A coordinate is not a number (a DataWeave <c>EXPRESSION</c> error, 400 in the flow).</exception>
    public PlaceView ToPlace()
    {
        // 'payload.places[0]' on a missing or empty array is null in DataWeave; every field of place is then null.
        var place = Places is { Count: > 0 } ? Places[0] : new ZippoPlaceWire();
        return new PlaceView(
            place.PlaceName,
            place.State,
            place.StateAbbreviation,
            ParseNumber(place.Latitude, "latitude"),
            ParseNumber(place.Longitude, "longitude"));
    }

    private static double? ParseNumber(string? value, string field)
    {
        if (value is null)
        {
            return null;
        }

        try
        {
            return double.Parse(value, NumberStyles.Float, CultureInfo.InvariantCulture);
        }
        catch (FormatException e)
        {
            throw new CoercionException(field, value, "Number", e);
        }
    }
}

/// <summary>One element of <c>places</c>.</summary>
public sealed record ZippoPlaceWire
{
    /// <summary>The <c>"place name"</c> field.</summary>
    [JsonPropertyName("place name")]
    public string? PlaceName { get; init; }

    /// <summary>The <c>"longitude"</c> field (a string on the wire).</summary>
    [JsonPropertyName("longitude")]
    public string? Longitude { get; init; }

    /// <summary>The <c>"state"</c> field.</summary>
    [JsonPropertyName("state")]
    public string? State { get; init; }

    /// <summary>The <c>"state abbreviation"</c> field.</summary>
    [JsonPropertyName("state abbreviation")]
    public string? StateAbbreviation { get; init; }

    /// <summary>The <c>"latitude"</c> field (a string on the wire).</summary>
    [JsonPropertyName("latitude")]
    public string? Latitude { get; init; }
}
