namespace Contacts.Core.Zippo;

/// <summary>The outbound side of <c>normalize-contacts-flow</c>: <c>&lt;http:request-config name="Zippopotam_Request_config"&gt;</c>.</summary>
public interface IZippoClient
{
    /// <summary>
    /// <c>&lt;http:request doc:name="GET /{country}/{postal-code}" method="GET" path="/{country}/{postalCode}"&gt;</c>; <paramref name="country"/> is lower-cased
    /// in the path as the <c>uri-params</c> expression did.
    /// </summary>
    /// <exception cref="PostalCodeNotFoundException">The service answered 404 (<c>HTTP:NOT_FOUND</c>).</exception>
    /// <exception cref="UpstreamException">Connection failure, timeout or a 500/502/503 answer.</exception>
    /// <exception cref="HttpRequestException">Any other non-success status.</exception>
    Task<ZippoWire> LookupAsync(string country, string postalCode, CancellationToken cancellationToken);
}

/// <summary>The Mule <c>HTTP:NOT_FOUND</c> error: the postal code is unknown to the service.</summary>
public sealed class PostalCodeNotFoundException : Exception
{
    /// <summary>Creates the exception for <paramref name="country"/>/<paramref name="postalCode"/>.</summary>
    public PostalCodeNotFoundException(string country, string postalCode, Exception? inner = null)
        : base($"Postal code '{postalCode}' not found for country '{country}'", inner)
    {
        Country = country;
        PostalCode = postalCode;
    }

    /// <summary>The country as sent in the path.</summary>
    public string Country { get; }

    /// <summary>The postal code as sent in the path.</summary>
    public string PostalCode { get; }
}

/// <summary>
/// The Mule <c>HTTP:CONNECTIVITY</c>, <c>HTTP:TIMEOUT</c>, <c>HTTP:INTERNAL_SERVER_ERROR</c>,
/// <c>HTTP:SERVICE_UNAVAILABLE</c> and <c>HTTP:BAD_GATEWAY</c> errors, mapped to 502 by the API.
/// </summary>
public sealed class UpstreamException : Exception
{
    /// <summary>Creates the exception with a description and the transport error, if any.</summary>
    public UpstreamException(string message, Exception? inner = null) : base(message, inner)
    {
    }
}
