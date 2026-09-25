using Contacts.Core.Data;
using Contacts.Core.Mapping;
using Contacts.Core.Settings;
using Contacts.Core.Zippo;
using Contacts.Tests.Support;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace Contacts.Tests.Api;

/// <summary>
/// The API hosted in process: a SQLite database seeded with <c>contact-data-100.csv</c> and the upstream client
/// replaced by <see cref="StubZippoClient"/>. <c>DATA_DIR</c> points at the temp directory while the fixture lives,
/// because the host opens its database from the environment before the test services replace it.
/// </summary>
public sealed class ApiFixture : IAsyncLifetime
{
    private readonly TempDir dir = new();

    public StubZippoClient Zippo { get; } = new();

    public WebApplicationFactory<Program> Factory { get; private set; } = null!;

    public HttpClient Client { get; private set; } = null!;

    public OpenApiContract Contract { get; } = new();

    public async Task InitializeAsync()
    {
        Environment.SetEnvironmentVariable("DATA_DIR", dir.Path);
        var repository = await SqlContactRepository.OpenAsync(DbProfile.Sqlite, $"Data Source={Path.Combine(dir.Path, "contacts.db")}", CancellationToken.None);
        var contacts = TestData.ReadRows("contact-data-100.csv").Select(row => ContactMapper.FromRow(row)).ToList();
        await repository.BulkInsertAsync(contacts, CancellationToken.None);

        Factory = new WebApplicationFactory<Program>().WithWebHostBuilder(builder => builder.ConfigureServices(services =>
        {
            services.RemoveAll<AppSettings>();
            services.AddSingleton(new AppSettings { DataDir = dir.Path, NormalizeMaxContacts = 50 });
            services.RemoveAll<IContactRepository>();
            services.AddSingleton<IContactRepository>(repository);
            services.RemoveAll<IZippoClient>();
            services.AddSingleton<IZippoClient>(Zippo);
        }));
        Client = Factory.CreateClient();
    }

    public Task DisposeAsync()
    {
        Client.Dispose();
        Factory.Dispose();
        Environment.SetEnvironmentVariable("DATA_DIR", null);
        dir.Dispose();
        return Task.CompletedTask;
    }
}
