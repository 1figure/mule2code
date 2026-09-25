using System.Net;
using System.Net.Http.Json;

namespace Contacts.Core.Zippo;

/// <summary><see cref="IZippoClient"/> on an <see cref="HttpClient"/> whose <see cref="HttpClient.BaseAddress"/> is <c>ZIPPOPOTAM_BASE_URL</c>.</summary>
public sealed class ZippoClient : IZippoClient
{
    private readonly HttpClient http;

    /// <summary>Creates the client; <paramref name="http"/> must have its base address set.</summary>
    public ZippoClient(HttpClient http)
    {
        ArgumentNullException.ThrowIfNull(http);
        if (http.BaseAddress is null)
        {
            throw new ArgumentException("HttpClient.BaseAddress must be set to the zippopotam.us base URL", nameof(http));
        }

        this.http = http;
    }

    /// <inheritdoc />
    public async Task<ZippoWire> LookupAsync(string country, string postalCode, CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(country);
        ArgumentNullException.ThrowIfNull(postalCode);
        var path = $"{Uri.EscapeDataString(country.ToLowerInvariant())}/{Uri.EscapeDataString(postalCode)}";
        HttpResponseMessage response;
        try
        {
            response = await http.GetAsync(path, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
        }
        catch (HttpRequestException e)
        {
            throw new UpstreamException($"HTTP GET on resource '{http.BaseAddress}{path}' failed: {e.Message}", e);
        }
        catch (TaskCanceledException e) when (!cancellationToken.IsCancellationRequested)
        {
            // HttpClient reports its own timeout as a TaskCanceledException.
            throw new UpstreamException($"HTTP GET on resource '{http.BaseAddress}{path}' timed out", e);
        }

        using (response)
        {
            switch (response.StatusCode)
            {
                case HttpStatusCode.NotFound:
                    throw new PostalCodeNotFoundException(country, postalCode);
                case HttpStatusCode.InternalServerError:
                case HttpStatusCode.BadGateway:
                case HttpStatusCode.ServiceUnavailable:
                    throw new UpstreamException($"HTTP GET on resource '{http.BaseAddress}{path}' failed: {(int)response.StatusCode} {response.ReasonPhrase}");
            }

            response.EnsureSuccessStatusCode();
            try
            {
                return await response.Content.ReadFromJsonAsync<ZippoWire>(cancellationToken).ConfigureAwait(false)
                    ?? throw new UpstreamException($"HTTP GET on resource '{http.BaseAddress}{path}' returned an empty body");
            }
            catch (System.Text.Json.JsonException e)
            {
                throw new UpstreamException($"HTTP GET on resource '{http.BaseAddress}{path}' returned malformed JSON: {e.Message}", e);
            }
        }
    }
}
