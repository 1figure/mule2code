using System.Net;
using Contacts.Core.Zippo;
using Contacts.Tests.Support;

namespace Contacts.Tests.Api;

/// <summary><see cref="ZippoClient"/> status mapping over a fake <see cref="HttpMessageHandler"/>.</summary>
public sealed class ZippoClientTests
{
    private sealed class FakeHandler : HttpMessageHandler
    {
        public Func<HttpRequestMessage, HttpResponseMessage> Respond { get; set; } = _ => new HttpResponseMessage(HttpStatusCode.OK);

        public List<Uri> Requests { get; } = [];

        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            Requests.Add(request.RequestUri!);
            return Task.FromResult(Respond(request));
        }
    }

    private static (ZippoClient Client, FakeHandler Handler) Create()
    {
        var handler = new FakeHandler();
        var http = new HttpClient(handler) { BaseAddress = new Uri("https://api.zippopotam.us/") };
        return (new ZippoClient(http), handler);
    }

    [Fact]
    public async Task LowerCasesTheCountryInThePathAndParsesTheBody()
    {
        var (client, handler) = Create();
        handler.Respond = _ => new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent(TestData.ReadText("api/zippopotam-us-63131.json")) };

        var wire = await client.LookupAsync("US", "63131", CancellationToken.None);

        Assert.Equal(new Uri("https://api.zippopotam.us/us/63131"), Assert.Single(handler.Requests));
        Assert.Equal("Saint Louis", wire.ToPlace().City);
    }

    [Fact]
    public async Task NotFoundIsPostalCodeNotFound()
    {
        var (client, handler) = Create();
        handler.Respond = _ => new HttpResponseMessage(HttpStatusCode.NotFound);

        var e = await Assert.ThrowsAsync<PostalCodeNotFoundException>(() => client.LookupAsync("US", "00000", CancellationToken.None));

        Assert.Equal("00000", e.PostalCode);
    }

    [Theory]
    [InlineData(HttpStatusCode.InternalServerError)]
    [InlineData(HttpStatusCode.BadGateway)]
    [InlineData(HttpStatusCode.ServiceUnavailable)]
    public async Task ServerErrorsAreUpstreamExceptions(HttpStatusCode status)
    {
        var (client, handler) = Create();
        handler.Respond = _ => new HttpResponseMessage(status);

        await Assert.ThrowsAsync<UpstreamException>(() => client.LookupAsync("US", "63131", CancellationToken.None));
    }

    [Fact]
    public async Task ConnectionFailureIsAnUpstreamExceptionWithTheCause()
    {
        var (client, handler) = Create();
        handler.Respond = _ => throw new HttpRequestException("Connection refused");

        var e = await Assert.ThrowsAsync<UpstreamException>(() => client.LookupAsync("US", "63131", CancellationToken.None));

        Assert.IsType<HttpRequestException>(e.InnerException);
        Assert.Contains("Connection refused", e.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task OtherStatusesAreNotMappedTo502()
    {
        var (client, handler) = Create();
        handler.Respond = _ => new HttpResponseMessage(HttpStatusCode.BadRequest);

        await Assert.ThrowsAsync<HttpRequestException>(() => client.LookupAsync("US", "63131", CancellationToken.None));
    }

    [Fact]
    public async Task MalformedBodyIsAnUpstreamException()
    {
        var (client, handler) = Create();
        handler.Respond = _ => new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent("{not json") };

        await Assert.ThrowsAsync<UpstreamException>(() => client.LookupAsync("US", "63131", CancellationToken.None));
    }

    [Fact]
    public void RequiresABaseAddress()
    {
        using var http = new HttpClient();

        Assert.Throws<ArgumentException>(() => new ZippoClient(http));
    }
}
