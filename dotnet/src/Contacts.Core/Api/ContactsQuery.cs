using System.Text.Json.Serialization;
using Contacts.Core.Data;

namespace Contacts.Core.Api;

/// <summary>The response body of <c>GET /contacts</c> (<c>ContactsByCity</c> in <c>openapi.yaml</c>).</summary>
public sealed record ContactsByCityView(
    [property: JsonPropertyName("city")] string City,
    [property: JsonPropertyName("count")] int Count,
    [property: JsonPropertyName("contacts")] IReadOnlyList<ContactView> Contacts);

/// <summary>One contact of <see cref="ContactsByCityView"/> (<c>ContactView</c> in <c>openapi.yaml</c>).</summary>
public sealed record ContactView(
    [property: JsonPropertyName("contact_id")] string? ContactId,
    [property: JsonPropertyName("first_name")] string? FirstName,
    [property: JsonPropertyName("last_name")] string? LastName,
    [property: JsonPropertyName("email")] string? Email,
    [property: JsonPropertyName("phone")] string? Phone,
    [property: JsonPropertyName("title")] string? Title,
    [property: JsonPropertyName("department")] string? Department,
    [property: JsonPropertyName("mailing")] MailingView Mailing);

/// <summary>The nested <c>mailing</c> object of <see cref="ContactView"/>.</summary>
public sealed record MailingView(
    [property: JsonPropertyName("street")] string? Street,
    [property: JsonPropertyName("city")] string? City,
    [property: JsonPropertyName("state")] string? State,
    [property: JsonPropertyName("postal_code")] string? PostalCode,
    [property: JsonPropertyName("country")] string? Country);

/// <summary>The validations and the DataWeave transform of <c>get-contacts-by-city-flow</c>.</summary>
public static class ContactsQuery
{
    /// <summary>Default of <c>attributes.queryParams.limit default '50'</c>.</summary>
    public const int DefaultLimit = 50;

    /// <summary>The <c>message</c> of <c>&lt;validation:is-not-blank-string doc:name="city is required"&gt;</c>.</summary>
    public const string CityRequiredMessage = "Query parameter 'city' is required";

    /// <summary>The <c>message</c> of <c>&lt;validation:is-number doc:name="limit in range" minValue="1" maxValue="500"&gt;</c>.</summary>
    public const string LimitRangeMessage = "Query parameter 'limit' must be between 1 and 500";

    /// <summary>
    /// The two validations of the flow. Returns the parsed <c>limit</c>, or the error message of the first failing check.
    /// </summary>
    /// <param name="city"><c>attributes.queryParams.city default ''</c>.</param>
    /// <param name="limit"><c>attributes.queryParams.limit default '50'</c>, unparsed.</param>
    /// <param name="parsedLimit">The limit as an integer when validation passed.</param>
    public static string? Validate(string city, string? limit, out int parsedLimit)
    {
        ArgumentNullException.ThrowIfNull(city);
        parsedLimit = DefaultLimit;

        // <validation:is-not-blank-string value="#[vars.city]">
        if (string.IsNullOrWhiteSpace(city))
        {
            return CityRequiredMessage;
        }

        // '(... default '50') as Number' followed by <validation:is-number numberType="INTEGER" minValue="1" maxValue="500">.
        // A non-numeric value fails the coercion (an EXPRESSION error in Mule); it is reported with the same message.
        var raw = string.IsNullOrEmpty(limit) ? DefaultLimit.ToString(System.Globalization.CultureInfo.InvariantCulture) : limit;
        if (!int.TryParse(raw, System.Globalization.NumberStyles.Integer, System.Globalization.CultureInfo.InvariantCulture, out var value)
            || value < 1 || value > 500)
        {
            return LimitRangeMessage;
        }

        parsedLimit = value;
        return null;
    }

    /// <summary><c>&lt;ee:transform doc:name="Rows to JSON"&gt;</c>.</summary>
    public static ContactsByCityView RowsToJson(string city, IReadOnlyList<ContactRow> rows)
    {
        ArgumentNullException.ThrowIfNull(city);
        ArgumentNullException.ThrowIfNull(rows);
        var contacts = rows.Select(row => new ContactView(
            row.ContactId,
            row.FirstName,
            row.LastName,
            row.Email,
            row.Phone,
            row.Title,
            row.Department,
            new MailingView(
                row.MailingStreet,
                row.MailingCity,
                row.MailingState,
                row.MailingPostalCode,
                row.MailingCountry))).ToList();
        return new ContactsByCityView(city, contacts.Count, contacts);
    }
}
