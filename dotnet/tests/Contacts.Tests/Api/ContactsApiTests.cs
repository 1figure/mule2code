using System.Net;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json.Nodes;
using Contacts.Core.Data;
using Contacts.Core.Model;
using Contacts.Core.Zippo;
using Contacts.Tests.Support;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace Contacts.Tests.Api;

/// <summary>The two HTTP flows through an in-process host, against <c>testdata/api/*.json</c> and <c>openapi.yaml</c>.</summary>
[Collection("process-environment")] // the host reads AppSettings.FromEnvironment() at start-up
public sealed class ContactsApiTests : IClassFixture<ApiFixture>
{
    private readonly ApiFixture fixture;

    public ContactsApiTests(ApiFixture fixture)
    {
        this.fixture = fixture;
        fixture.Zippo.Calls.Clear();
        fixture.Zippo.Failure = null;
        fixture.Zippo.Canned.Clear();
    }

    private HttpClient Client => fixture.Client;

    private static StringContent JsonBody(string json) => new(json, Encoding.UTF8, "application/json");

    private async Task<(HttpStatusCode Status, string Body)> PostNormalizeAsync(HttpContent content)
    {
        using var response = await Client.PostAsync(new Uri("/contacts/normalize", UriKind.Relative), content);
        return (response.StatusCode, await response.Content.ReadAsStringAsync());
    }

    private Task<(HttpStatusCode Status, string Body)> PostNormalizeAsync(string json) => PostNormalizeAsync(JsonBody(json));

    private async Task<(HttpStatusCode Status, string Body)> GetAsync(string url)
    {
        using var response = await Client.GetAsync(new Uri(url, UriKind.Relative));
        return (response.StatusCode, await response.Content.ReadAsStringAsync());
    }

    [Fact]
    public async Task GetContactsBySaintLouisMatchesTheGolden()
    {
        var (status, body) = await GetAsync("/contacts?city=Saint%20Louis");

        Assert.Equal(HttpStatusCode.OK, status);
        Assert.Equal(Json.Canonical(TestData.ReadText("api/get-contacts-saint-louis.json")), Json.Canonical(body));
        fixture.Contract.AssertConforms(body, "ContactsByCity");
    }

    [Fact]
    public async Task GetContactsIsCaseInsensitiveOnCityAndEchoesTheQueryValue()
    {
        var (status, body) = await GetAsync("/contacts?city=saint%20louis");

        Assert.Equal(HttpStatusCode.OK, status);
        var node = JsonNode.Parse(body)!;
        Assert.Equal("saint louis", node["city"]!.GetValue<string>());
        Assert.Equal(2, node["count"]!.GetValue<int>());
    }

    [Fact]
    public async Task GetContactsHonoursTheLimit()
    {
        var (status, body) = await GetAsync("/contacts?city=Saint%20Louis&limit=1");

        Assert.Equal(HttpStatusCode.OK, status);
        var node = JsonNode.Parse(body)!;
        Assert.Equal(1, node["count"]!.GetValue<int>());
        Assert.Equal("003Hu0000000054IAA", node["contacts"]![0]!["contact_id"]!.GetValue<string>());
        fixture.Contract.AssertConforms(body, "ContactsByCity");
    }

    [Fact]
    public async Task GetContactsForAnUnknownCityIsAnEmptyList()
    {
        var (status, body) = await GetAsync("/contacts?city=Atlantis");

        Assert.Equal(HttpStatusCode.OK, status);
        Assert.Equal(0, JsonNode.Parse(body)!["count"]!.GetValue<int>());
        fixture.Contract.AssertConforms(body, "ContactsByCity");
    }

    [Theory]
    [InlineData("/contacts", "Query parameter 'city' is required")]
    [InlineData("/contacts?city=", "Query parameter 'city' is required")]
    [InlineData("/contacts?city=%20%20", "Query parameter 'city' is required")]
    [InlineData("/contacts?city=x&limit=0", "Query parameter 'limit' must be between 1 and 500")]
    [InlineData("/contacts?city=x&limit=501", "Query parameter 'limit' must be between 1 and 500")]
    [InlineData("/contacts?city=x&limit=abc", "Query parameter 'limit' must be between 1 and 500")]
    [InlineData("/contacts?city=x&limit=1.5", "Query parameter 'limit' must be between 1 and 500")]
    public async Task GetContactsValidationFailuresAre400WithTheXmlMessage(string url, string message)
    {
        var (status, body) = await GetAsync(url);

        Assert.Equal(HttpStatusCode.BadRequest, status);
        Assert.Equal(message, JsonNode.Parse(body)!["error"]!.GetValue<string>());
        fixture.Contract.AssertErrorBody(body);
    }

    [Fact]
    public async Task GetContactsDatabaseFailureIs503WithTheDescription()
    {
        // <on-error-propagate type="DB:CONNECTIVITY, DB:QUERY_EXECUTION"> on a host whose repository is down.
        using var factory = fixture.Factory.WithWebHostBuilder(b => b.ConfigureServices(s =>
        {
            s.RemoveAll<IContactRepository>();
            s.AddSingleton<IContactRepository>(new ThrowingRepository());
        }));
        using var client = factory.CreateClient();

        using var response = await client.GetAsync(new Uri("/contacts?city=Saint%20Louis", UriKind.Relative));
        var body = await response.Content.ReadAsStringAsync();

        Assert.Equal(HttpStatusCode.ServiceUnavailable, response.StatusCode);
        Assert.Equal("Select contacts by city failed: connection refused", JsonNode.Parse(body)!["error"]!.GetValue<string>());
        fixture.Contract.AssertErrorBody(body);
    }

    [Fact]
    public async Task PostNormalizeMatchesTheGolden()
    {
        var (status, body) = await PostNormalizeAsync(TestData.ReadText("api/normalize-request.json"));

        Assert.Equal(HttpStatusCode.OK, status);
        Assert.Equal(Json.Canonical(TestData.ReadText("api/normalize-response.json")), Json.Canonical(body));
        Assert.Equal([("US", "63131"), ("us", "91499"), ("US", "00000")], fixture.Zippo.Calls);
        fixture.Contract.AssertConforms(body, "NormalizeResponse");
    }

    [Theory]
    [InlineData("{}", "Body must contain a non-empty 'contacts' array")]
    [InlineData("{\"contacts\": []}", "Body must contain a non-empty 'contacts' array")]
    [InlineData("{\"contacts\": [{\"contact_id\": \"x\", \"mailing_postal_code\": \"63131\"}]}", "Every contact needs 'mailing_country'")]
    [InlineData("{\"contacts\": [{\"contact_id\": \"x\", \"mailing_postal_code\": \"63131\", \"mailing_country\": \"\"}]}", "Every contact needs 'mailing_country'")]
    [InlineData("{\"contacts\": [{\"contact_id\": \"x\"}]}", "Every contact needs 'mailing_postal_code'\nEvery contact needs 'mailing_country'")]
    public async Task PostNormalizeValidationFailuresAre400WithTheXmlMessage(string json, string message)
    {
        var (status, body) = await PostNormalizeAsync(json);

        Assert.Equal(HttpStatusCode.BadRequest, status);
        Assert.Equal(message, JsonNode.Parse(body)!["error"]!.GetValue<string>());
        Assert.Empty(fixture.Zippo.Calls);
        fixture.Contract.AssertErrorBody(body);
    }

    [Fact]
    public async Task PostNormalizeWithTheInvalidFixtureIs400BeforeAnyUpstreamCall()
    {
        var (status, body) = await PostNormalizeAsync(TestData.ReadText("api/normalize-request-invalid.json"));

        Assert.Equal(HttpStatusCode.BadRequest, status);
        Assert.Equal("Every contact needs 'mailing_postal_code'", JsonNode.Parse(body)!["error"]!.GetValue<string>());
        Assert.Empty(fixture.Zippo.Calls);
    }

    [Fact]
    public async Task PostNormalizeWithTooManyContactsIs400BeforeAnyUpstreamCall()
    {
        var contacts = string.Join(',', Enumerable.Range(0, 51).Select(i => $"{{\"contact_id\": \"{i}\", \"mailing_postal_code\": \"63131\", \"mailing_country\": \"US\"}}"));

        var (status, body) = await PostNormalizeAsync($"{{\"contacts\": [{contacts}]}}");

        Assert.Equal(HttpStatusCode.BadRequest, status);
        Assert.Equal("At most 50 contacts per request", JsonNode.Parse(body)!["error"]!.GetValue<string>());
        Assert.Empty(fixture.Zippo.Calls);
    }

    [Fact]
    public async Task PostNormalizeWithMalformedJsonIs400()
    {
        var (status, body) = await PostNormalizeAsync("{\"contacts\": [");

        Assert.Equal(HttpStatusCode.BadRequest, status);
        fixture.Contract.AssertErrorBody(body);
        Assert.Empty(fixture.Zippo.Calls);
    }

    [Fact]
    public async Task PostNormalizeWithAnEmptyBodyIsTheContactsPresentMessage()
    {
        // 'payload.contacts default []' on no payload → the first validation fails.
        var (status, body) = await PostNormalizeAsync(new StringContent("", Encoding.UTF8, "application/json"));

        Assert.Equal(HttpStatusCode.BadRequest, status);
        Assert.Equal("Body must contain a non-empty 'contacts' array", JsonNode.Parse(body)!["error"]!.GetValue<string>());
        Assert.Empty(fixture.Zippo.Calls);
    }

    [Fact]
    public async Task PostNormalizeWithANonJsonContentTypeIs400()
    {
        var (status, body) = await PostNormalizeAsync(new StringContent("contacts=1", Encoding.UTF8, "text/plain"));

        Assert.Equal(HttpStatusCode.BadRequest, status);
        fixture.Contract.AssertErrorBody(body);
        Assert.Empty(fixture.Zippo.Calls);
    }

    [Fact]
    public async Task PostNormalizeWithNonNumericCoordinatesIs400()
    {
        // 'latitude as Number' failing in "Zippopotam to result" is an EXPRESSION error, which the flow maps to 400.
        fixture.Zippo.Canned[("US", "63131")] = new ZippoWire
        {
            Places = [new ZippoPlaceWire { PlaceName = "Saint Louis", State = "Missouri", StateAbbreviation = "MO", Latitude = "north", Longitude = "0.0" }],
        };

        var (status, body) = await PostNormalizeAsync(TestData.ReadText("api/normalize-request.json"));

        Assert.Equal(HttpStatusCode.BadRequest, status);
        Assert.Equal("Cannot coerce String (north) to Number", JsonNode.Parse(body)!["error"]!.GetValue<string>());
        fixture.Contract.AssertErrorBody(body);
    }

    [Fact]
    public async Task PostNormalizeWithNoPlacesIsOkWithNullPlaceFields()
    {
        // 'payload.places[0]' is null in DataWeave when the array is missing; the result is still status "ok".
        fixture.Zippo.Canned[("US", "63131")] = new ZippoWire { Places = null };

        var (status, body) = await PostNormalizeAsync("{\"contacts\": [{\"contact_id\": \"x\", \"mailing_postal_code\": \"63131\", \"mailing_country\": \"US\"}]}");

        Assert.Equal(HttpStatusCode.OK, status);
        var result = JsonNode.Parse(body)!["results"]![0]!;
        Assert.Equal("ok", result["status"]!.GetValue<string>());
        Assert.NotNull(result["place"]);
        Assert.Null(result["place"]!["city"]);
        Assert.Null(result["place"]!["latitude"]);
    }

    [Fact]
    public async Task PostNormalizeUpstreamFailureIs502WithTheDescription()
    {
        const string description = "HTTP GET on resource 'https://api.zippopotam.us/us/63131' failed: Connection refused";
        fixture.Zippo.Failure = new UpstreamException(description);

        var (status, body) = await PostNormalizeAsync(TestData.ReadText("api/normalize-request.json"));

        Assert.Equal(HttpStatusCode.BadGateway, status);
        Assert.Equal(description, JsonNode.Parse(body)!["error"]!.GetValue<string>());
        fixture.Contract.AssertErrorBody(body);
    }

    [Fact]
    public async Task UnexpectedErrorIs500WithTheSameBodyShape()
    {
        fixture.Zippo.Failure = new HttpRequestException("418 I'm a teapot");

        var (status, body) = await PostNormalizeAsync(TestData.ReadText("api/normalize-request.json"));

        Assert.Equal(HttpStatusCode.InternalServerError, status);
        Assert.Contains("teapot", JsonNode.Parse(body)!["error"]!.GetValue<string>(), StringComparison.Ordinal);
        fixture.Contract.AssertErrorBody(body);
    }

    [Fact]
    public async Task OnlyTheDeclaredMethodsAreRouted()
    {
        using var post = await Client.PostAsync(new Uri("/contacts", UriKind.Relative), JsonBody("{}"));
        using var get = await Client.GetAsync(new Uri("/contacts/normalize", UriKind.Relative));

        Assert.Equal(HttpStatusCode.MethodNotAllowed, post.StatusCode);
        Assert.Equal(HttpStatusCode.MethodNotAllowed, get.StatusCode);
    }

    [Fact]
    public async Task ResponsesAreJson()
    {
        using var response = await Client.GetAsync(new Uri("/contacts?city=Saint%20Louis", UriKind.Relative));

        Assert.Equal("application/json", response.Content.Headers.ContentType?.MediaType);
        Assert.NotNull(await response.Content.ReadFromJsonAsync<JsonNode>());
    }

    private sealed class ThrowingRepository : IContactRepository
    {
        public Task BulkInsertAsync(IReadOnlyList<Contact> contacts, CancellationToken cancellationToken) =>
            throw new RepositoryException("Bulk insert failed: connection refused", new TimeoutException());

        public Task<IReadOnlyList<ContactRow>> QueryByCityAsync(string city, int limit, CancellationToken cancellationToken) =>
            throw new RepositoryException("Select contacts by city failed: connection refused", new TimeoutException());
    }
}
