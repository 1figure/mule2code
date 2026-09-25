using System.Text.Json;
using Contacts.Core.Zippo;

namespace Contacts.Tests.Support;

/// <summary>
/// Answers from <c>testdata/api/zippopotam-{country}-{postalCode}.json</c>; a postal code without a fixture is a 404.
/// Records every call and can be switched to fail like an unreachable upstream.
/// </summary>
public sealed class StubZippoClient : IZippoClient
{
    public List<(string Country, string PostalCode)> Calls { get; } = [];

    public Exception? Failure { get; set; }

    /// <summary>Answers that take precedence over the fixture files, keyed by (country as sent, postal code).</summary>
    public Dictionary<(string Country, string PostalCode), ZippoWire> Canned { get; } = [];

    public Task<ZippoWire> LookupAsync(string country, string postalCode, CancellationToken cancellationToken)
    {
        Calls.Add((country, postalCode));
        if (Failure != null)
        {
            throw Failure;
        }

        if (Canned.TryGetValue((country, postalCode), out var canned))
        {
            return Task.FromResult(canned);
        }

        var path = TestData.Fixture($"api/zippopotam-{country.ToLowerInvariant()}-{postalCode}.json");
        if (!File.Exists(path))
        {
            throw new PostalCodeNotFoundException(country, postalCode);
        }

        var wire = JsonSerializer.Deserialize<ZippoWire>(File.ReadAllText(path))
            ?? throw new InvalidDataException(path);
        return Task.FromResult(wire);
    }
}
