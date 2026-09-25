using System.Text.Json;
using Contacts.Core.Api;
using Contacts.Core.Data;
using Contacts.Core.Mapping;
using Contacts.Core.Settings;
using Contacts.Core.Zippo;
using Microsoft.AspNetCore.Http.HttpResults;

namespace Contacts.Api;

/// <summary>The two flows of <c>contacts-api-impl.xml</c> as minimal-API handlers.</summary>
public static class ContactsEndpoints
{
    private const string LoggerCategory = "Contacts.Api.ContactsEndpoints";

    /// <summary>Registers <c>GET /contacts</c> and <c>POST /contacts/normalize</c> on <paramref name="app"/>.</summary>
    public static IEndpointRouteBuilder MapContacts(this IEndpointRouteBuilder app)
    {
        ArgumentNullException.ThrowIfNull(app);
        // <http:listener doc:name="GET /contacts" path="/contacts" allowedMethods="GET">
        app.MapGet("/contacts", GetContactsByCityAsync);
        // <http:listener doc:name="POST /contacts/normalize" path="/contacts/normalize" allowedMethods="POST">
        app.MapPost("/contacts/normalize", NormalizeContactsAsync);
        return app;
    }

    // <flow name="get-contacts-by-city-flow">
    private static async Task<Results<Ok<ContactsByCityView>, JsonHttpResult<ErrorBody>>> GetContactsByCityAsync(
        HttpRequest request,
        IContactRepository repository,
        ILoggerFactory loggerFactory,
        CancellationToken cancellationToken)
    {
        // <set-variable variableName="city" value="#[attributes.queryParams.city default '']">
        var city = request.Query["city"].FirstOrDefault() ?? "";
        // <set-variable variableName="limit" value="#[(attributes.queryParams.limit default '50') as Number]">
        var limit = request.Query["limit"].FirstOrDefault();

        // <on-error-propagate type="VALIDATION:INVALID_STRING, VALIDATION:INVALID_NUMBER, VALIDATION:NOT_A_NUMBER"> → 400
        if (ContactsQuery.Validate(city, limit, out var parsedLimit) is { } message)
        {
            return Error(StatusCodes.Status400BadRequest, message);
        }

        try
        {
            // <db:select doc:name="Select contacts by city">
            var rows = await repository.QueryByCityAsync(city, parsedLimit, cancellationToken);
            // <ee:transform doc:name="Rows to JSON">
            return TypedResults.Ok(ContactsQuery.RowsToJson(city, rows));
        }
        catch (RepositoryException e)
        {
            // <on-error-propagate type="DB:CONNECTIVITY, DB:QUERY_EXECUTION"> → 503, <logger message="Database error: ...">
            loggerFactory.CreateLogger(LoggerCategory).LogError(e, "Database error: {Description}", e.Message);
            return Error(StatusCodes.Status503ServiceUnavailable, e.Message);
        }
    }

    // <flow name="normalize-contacts-flow">
    private static async Task<Results<Ok<NormalizeResponse>, JsonHttpResult<ErrorBody>>> NormalizeContactsAsync(
        HttpRequest request,
        IZippoClient zippo,
        AppSettings settings,
        ILoggerFactory loggerFactory,
        CancellationToken cancellationToken)
    {
        // The listener only parses a JSON body; anything else fails 'payload.contacts' with an EXPRESSION error → 400.
        if (request.ContentLength is > 0 && !request.HasJsonContentType())
        {
            return Error(StatusCodes.Status400BadRequest, "Request body must be application/json");
        }

        NormalizeRequest? body = null;
        if (request.ContentLength is null or > 0)
        {
            try
            {
                body = await request.ReadFromJsonAsync<NormalizeRequest>(cancellationToken);
            }
            catch (JsonException e)
            {
                return Error(StatusCodes.Status400BadRequest, $"Invalid JSON body: {e.Message}");
            }
        }

        // <set-variable variableName="contacts" value="#[payload.contacts default []]">
        var contacts = body?.Contacts ?? [];

        // <on-error-propagate type="VALIDATION:INVALID_BOOLEAN, VALIDATION:MULTIPLE, EXPRESSION"> → 400
        if (Normalize.Validate(contacts, settings.NormalizeMaxContacts) is { } message)
        {
            return Error(StatusCodes.Status400BadRequest, message);
        }

        // <set-variable variableName="results" value="#[[]]">
        var results = new List<NormalizeResult>(contacts.Count);
        try
        {
            // <foreach doc:name="For each contact" collection="#[vars.contacts]">
            foreach (var contact in contacts)
            {
                // <try doc:name="Try lookup"> ... <http:request doc:name="GET /{country}/{postal-code}">
                NormalizeResult result;
                try
                {
                    var wire = await zippo.LookupAsync(contact.MailingCountry ?? "", contact.MailingPostalCode ?? "", cancellationToken);
                    result = Normalize.ZippoToResult(contact, wire);
                }
                catch (PostalCodeNotFoundException)
                {
                    // <on-error-continue type="HTTP:NOT_FOUND" doc:name="404 -> not_found">
                    result = Normalize.NotFoundResult(contact);
                }

                // <set-variable variableName="results" value="#[vars.results + payload]">
                results.Add(result);
            }
        }
        catch (UpstreamException e)
        {
            // <on-error-propagate type="HTTP:CONNECTIVITY, HTTP:TIMEOUT, HTTP:INTERNAL_SERVER_ERROR, HTTP:SERVICE_UNAVAILABLE, HTTP:BAD_GATEWAY"> → 502
            // The "Upstream error: " prefix belongs to the <logger> only; the body is {error: error.description}.
            loggerFactory.CreateLogger(LoggerCategory).LogError(e, "Upstream error: {Description}", e.Message);
            return Error(StatusCodes.Status502BadGateway, e.Message);
        }
        catch (CoercionException e)
        {
            // 'latitude as Number' failing inside "Zippopotam to result" is an EXPRESSION error → the 400 handler above.
            return Error(StatusCodes.Status400BadRequest, e.Message);
        }

        // <ee:transform doc:name="Results to JSON">
        return TypedResults.Ok(Normalize.ResultsToJson(results));
    }

    // <http:error-response statusCode="#[vars.httpStatus default 500]"><http:body>{error: error.description}</http:body>
    private static JsonHttpResult<ErrorBody> Error(int status, string description) =>
        TypedResults.Json(new ErrorBody(description), statusCode: status);
}
