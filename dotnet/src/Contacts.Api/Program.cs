using System.Text.Json;
using Contacts.Api;
using Contacts.Core.Data;
using Contacts.Core.Settings;
using Contacts.Core.Zippo;

// <global-property name="env"> + properties file → environment variables.
var settings = AppSettings.FromEnvironment();

var builder = WebApplication.CreateBuilder(args);

// <http:listener-config name="HTTP_Listener_config"><http:listener-connection host= port=>
builder.WebHost.UseUrls(settings.HttpUrl);

builder.Services.AddSingleton(settings);

// <db:config name="Contacts_Database_Config">: opened before the host starts, so an unreachable database fails
// deployment the way it did in Mule.
var repository = await SqlContactRepository.OpenAsync(settings.DbProfile, settings.ConnectionString, CancellationToken.None);
builder.Services.AddSingleton<IContactRepository>(repository);

// <http:request-config name="Zippopotam_Request_config">
builder.Services.AddHttpClient<IZippoClient, ZippoClient>(http =>
{
    http.BaseAddress = new Uri(settings.ZippopotamBaseUrl.TrimEnd('/') + "/");
    http.Timeout = TimeSpan.FromSeconds(30);
});

var app = builder.Build();

// Any error the flows do not map themselves → <http:error-response statusCode="#[vars.httpStatus default 500]">.
app.UseExceptionHandler(errors => errors.Run(async context =>
{
    var feature = context.Features.Get<Microsoft.AspNetCore.Diagnostics.IExceptionHandlerFeature>();
    var description = feature?.Error.Message ?? "Internal server error";
    context.Response.StatusCode = StatusCodes.Status500InternalServerError;
    context.Response.ContentType = "application/json";
    await JsonSerializer.SerializeAsync(context.Response.Body, new ErrorBody(description), cancellationToken: context.RequestAborted);
}));

app.MapContacts();

await app.RunAsync();

/// <summary>Makes the entry point visible to the test host (<c>WebApplicationFactory&lt;Program&gt;</c>).</summary>
public partial class Program
{
}
