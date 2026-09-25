using System.Text.Json;
using Contacts.Core.Api;
using Contacts.Core.Zippo;
using Contacts.Tests.Support;

namespace Contacts.Tests.Golden;

/// <summary>The DataWeave transforms and validations of <c>contacts-api-impl.xml</c> against <c>testdata/api/*.json</c>.</summary>
public sealed class ApiTransformTests
{
    private static NormalizeRequest Request(string relative) =>
        JsonSerializer.Deserialize<NormalizeRequest>(TestData.ReadText(relative))!;

    [Fact]
    public void ZippopotamToResultMatchesTheGoldenFirstResult()
    {
        var contact = Request("api/normalize-request.json").Contacts![0];
        var wire = JsonSerializer.Deserialize<ZippoWire>(TestData.ReadText("api/zippopotam-us-63131.json"))!;

        var result = Normalize.ZippoToResult(contact, wire);

        Assert.Equal(Json.Canonical(TestData.ReadJson("api/normalize-response.json")["results"]![0]), Json.Canonical(result));
    }

    [Fact]
    public void ZippopotamToResultUpperCasesTheCountry()
    {
        var contact = Request("api/normalize-request.json").Contacts![1];
        var wire = JsonSerializer.Deserialize<ZippoWire>(TestData.ReadText("api/zippopotam-us-91499.json"))!;

        var result = Normalize.ZippoToResult(contact, wire);

        Assert.Equal("us", contact.MailingCountry);
        Assert.Equal(Json.Canonical(TestData.ReadJson("api/normalize-response.json")["results"]![1]), Json.Canonical(result));
    }

    [Fact]
    public void NotFoundResultMatchesTheGoldenThirdResult()
    {
        var contact = Request("api/normalize-request.json").Contacts![2];

        var result = Normalize.NotFoundResult(contact);

        Assert.Equal(Json.Canonical(TestData.ReadJson("api/normalize-response.json")["results"]![2]), Json.Canonical(result));
    }

    [Fact]
    public void ResultsToJsonCountsTheResults()
    {
        var results = Request("api/normalize-request.json").Contacts!.Select(Normalize.NotFoundResult).ToList();

        var response = Normalize.ResultsToJson(results);

        Assert.Equal(3, response.Count);
        Assert.Same(results, response.Results);
    }

    [Fact]
    public void ToPlaceParsesCoordinatesAsNumbers()
    {
        // The expected numbers come from the golden response, never from a literal: api.zippopotam.us has
        // changed these coordinates once already and the fixtures were refreshed from the live answer.
        var wire = JsonSerializer.Deserialize<ZippoWire>(TestData.ReadText("api/zippopotam-us-63131.json"))!;
        var expected = TestData.ReadJson("api/normalize-response.json")["results"]![0]!["place"]!;

        var place = wire.ToPlace();

        Assert.Equal(expected["latitude"]!.GetValue<double>(), place.Latitude);
        Assert.Equal(expected["longitude"]!.GetValue<double>(), place.Longitude);
        Assert.Equal(double.Parse(wire.Places![0].Latitude!, System.Globalization.CultureInfo.InvariantCulture), place.Latitude);
        Assert.Equal(expected["city"]!.GetValue<string>(), place.City);
        Assert.Equal(expected["state_abbreviation"]!.GetValue<string>(), place.StateAbbreviation);
    }

    [Fact]
    public void ToPlaceRejectsNonNumericCoordinatesAsACoercionError()
    {
        var wire = new ZippoWire { Places = [new ZippoPlaceWire { Latitude = "1.5", Longitude = "west" }] };

        var e = Assert.Throws<Contacts.Core.Mapping.CoercionException>(() => wire.ToPlace());

        Assert.Equal("longitude", e.Field);
        Assert.Equal("west", e.Value);
        Assert.Equal("Cannot coerce String (west) to Number", e.Message);
    }

    [Fact]
    public void ToPlaceOfAMissingOrEmptyPlacesArrayIsAllNulls()
    {
        Assert.Null(new ZippoWire { Places = [] }.ToPlace().City);
        Assert.Null(new ZippoWire { Places = null }.ToPlace().Latitude);
        Assert.Null(JsonSerializer.Deserialize<ZippoWire>("{\"places\": null}")!.ToPlace().State);
        Assert.Null(JsonSerializer.Deserialize<ZippoWire>("{}")!.ToPlace().Longitude);
    }

    [Fact]
    public void NormalizeValidationsUseTheMessagesFromTheXml()
    {
        var ok = Request("api/normalize-request.json").Contacts!;
        var invalid = Request("api/normalize-request-invalid.json").Contacts!;

        Assert.Null(Normalize.Validate(ok, 50));
        Assert.Equal("Body must contain a non-empty 'contacts' array", Normalize.Validate([], 50));
        Assert.Equal("At most 2 contacts per request", Normalize.Validate(ok, 2));
        Assert.Equal("Every contact needs 'mailing_postal_code'", Normalize.Validate(invalid, 50));
        Assert.Equal("Every contact needs 'mailing_country'", Normalize.Validate([new NormalizeContact { MailingPostalCode = "1", MailingCountry = " " }], 50));
        Assert.Equal(
            "Every contact needs 'mailing_postal_code'\nEvery contact needs 'mailing_country'",
            Normalize.Validate([new NormalizeContact()], 50));
    }

    [Fact]
    public void ContactsQueryValidationsUseTheMessagesFromTheXml()
    {
        Assert.Equal("Query parameter 'city' is required", ContactsQuery.Validate("", "10", out _));
        Assert.Equal("Query parameter 'city' is required", ContactsQuery.Validate("  ", null, out _));
        Assert.Equal("Query parameter 'limit' must be between 1 and 500", ContactsQuery.Validate("x", "0", out _));
        Assert.Equal("Query parameter 'limit' must be between 1 and 500", ContactsQuery.Validate("x", "501", out _));
        Assert.Equal("Query parameter 'limit' must be between 1 and 500", ContactsQuery.Validate("x", "abc", out _));

        Assert.Null(ContactsQuery.Validate("x", null, out var limit));
        Assert.Equal(50, limit);
        Assert.Null(ContactsQuery.Validate("x", "500", out limit));
        Assert.Equal(500, limit);
    }
}
