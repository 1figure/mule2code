using System.Globalization;
using System.Text.Json.Serialization;
using Contacts.Core.Zippo;

namespace Contacts.Core.Api;

/// <summary>The request body of <c>POST /contacts/normalize</c> (<c>NormalizeRequest</c> in <c>openapi.yaml</c>).</summary>
public sealed record NormalizeRequest
{
    /// <summary>The <c>contacts</c> array; <c>payload.contacts default []</c>.</summary>
    [JsonPropertyName("contacts")]
    public IReadOnlyList<NormalizeContact>? Contacts { get; init; }
}

/// <summary>One input contact of <see cref="NormalizeRequest"/>.</summary>
public sealed record NormalizeContact
{
    /// <summary>Echoed into the result; may be absent.</summary>
    [JsonPropertyName("contact_id")]
    public string? ContactId { get; init; }

    /// <summary>The postal code to look up.</summary>
    [JsonPropertyName("mailing_postal_code")]
    public string? MailingPostalCode { get; init; }

    /// <summary>ISO 3166-1 alpha-2 country, any case.</summary>
    [JsonPropertyName("mailing_country")]
    public string? MailingCountry { get; init; }
}

/// <summary>The <c>place</c> object of a result; <c>null</c> when the postal code was not found.</summary>
public sealed record PlaceView(
    [property: JsonPropertyName("city")] string? City,
    [property: JsonPropertyName("state")] string? State,
    [property: JsonPropertyName("state_abbreviation")] string? StateAbbreviation,
    [property: JsonPropertyName("latitude")] double? Latitude,
    [property: JsonPropertyName("longitude")] double? Longitude);

/// <summary>One element of <see cref="NormalizeResponse.Results"/>.</summary>
public sealed record NormalizeResult(
    [property: JsonPropertyName("contact_id")] string? ContactId,
    [property: JsonPropertyName("postal_code")] string? PostalCode,
    [property: JsonPropertyName("country")] string? Country,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("place")] PlaceView? Place);

/// <summary>The response body of <c>POST /contacts/normalize</c> (<c>NormalizeResponse</c> in <c>openapi.yaml</c>).</summary>
public sealed record NormalizeResponse(
    [property: JsonPropertyName("count")] int Count,
    [property: JsonPropertyName("results")] IReadOnlyList<NormalizeResult> Results);

/// <summary>The validations and the DataWeave transforms of <c>normalize-contacts-flow</c>.</summary>
public static class Normalize
{
    /// <summary>The <c>message</c> of <c>&lt;validation:is-true doc:name="contacts present"&gt;</c>.</summary>
    public const string ContactsPresentMessage = "Body must contain a non-empty 'contacts' array";

    /// <summary>The first <c>message</c> inside <c>&lt;validation:all&gt;</c>.</summary>
    public const string PostalCodeRequiredMessage = "Every contact needs 'mailing_postal_code'";

    /// <summary>The second <c>message</c> inside <c>&lt;validation:all&gt;</c>.</summary>
    public const string CountryRequiredMessage = "Every contact needs 'mailing_country'";

    /// <summary>The <c>message</c> of <c>&lt;validation:is-true doc:name="contacts not too many"&gt;</c>: <c>'At most ' ++ p('normalize.max_contacts') ++ ' contacts per request'</c>.</summary>
    public static string TooManyContactsMessage(int maxContacts) =>
        $"At most {maxContacts.ToString(CultureInfo.InvariantCulture)} contacts per request";

    /// <summary>
    /// The three validations of the flow, in order, all before any upstream call. Returns the error message of the
    /// first failing one, or <c>null</c>. <c>&lt;validation:all&gt;</c> reports every failing check of its group,
    /// joined by newlines (a <c>VALIDATION:MULTIPLE</c> error).
    /// </summary>
    public static string? Validate(IReadOnlyList<NormalizeContact> contacts, int maxContacts)
    {
        ArgumentNullException.ThrowIfNull(contacts);

        // <validation:is-true expression="#[sizeOf(vars.contacts) > 0]">
        if (contacts.Count == 0)
        {
            return ContactsPresentMessage;
        }

        // <validation:is-true expression="#[sizeOf(vars.contacts) <= (p('normalize.max_contacts') as Number)]">
        if (contacts.Count > maxContacts)
        {
            return TooManyContactsMessage(maxContacts);
        }

        // <validation:all doc:name="each contact has postal code and country">
        var failures = new List<string>();
        if (!contacts.All(c => !string.IsNullOrWhiteSpace(c.MailingPostalCode)))
        {
            failures.Add(PostalCodeRequiredMessage);
        }

        if (!contacts.All(c => !string.IsNullOrWhiteSpace(c.MailingCountry)))
        {
            failures.Add(CountryRequiredMessage);
        }

        return failures.Count == 0 ? null : string.Join("\n", failures);
    }

    /// <summary><c>&lt;ee:transform doc:name="Zippopotam to result"&gt;</c>: <c>status: "ok"</c> with <c>places[0]</c>.</summary>
    public static NormalizeResult ZippoToResult(NormalizeContact contact, ZippoWire wire)
    {
        ArgumentNullException.ThrowIfNull(contact);
        ArgumentNullException.ThrowIfNull(wire);
        return new NormalizeResult(
            contact.ContactId,
            contact.MailingPostalCode,
            contact.MailingCountry?.ToUpperInvariant(),
            "ok",
            wire.ToPlace());
    }

    /// <summary><c>&lt;ee:transform doc:name="Not found result"&gt;</c> inside <c>&lt;on-error-continue type="HTTP:NOT_FOUND"&gt;</c>.</summary>
    public static NormalizeResult NotFoundResult(NormalizeContact contact)
    {
        ArgumentNullException.ThrowIfNull(contact);
        return new NormalizeResult(
            contact.ContactId,
            contact.MailingPostalCode,
            contact.MailingCountry?.ToUpperInvariant(),
            "not_found",
            null);
    }

    /// <summary><c>&lt;ee:transform doc:name="Results to JSON"&gt;</c>.</summary>
    public static NormalizeResponse ResultsToJson(IReadOnlyList<NormalizeResult> results)
    {
        ArgumentNullException.ThrowIfNull(results);
        return new NormalizeResponse(results.Count, results);
    }
}
