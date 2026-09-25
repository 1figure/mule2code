using Contacts.Core.Api;
using Contacts.Core.Data;
using Contacts.Core.Mapping;
using Contacts.Tests.Support;

namespace Contacts.Tests.Data;

/// <summary>
/// A repository loaded once with <c>contact-data-100.csv</c>; the contract tests below are read-only against it.
/// <see cref="SkipReason"/> is set when the backing database is unavailable.
/// </summary>
public abstract class RepositoryFixture : IAsyncLifetime
{
    public IContactRepository Repository { get; private set; } = null!;

    public string? SkipReason { get; protected set; }

    public async Task InitializeAsync()
    {
        try
        {
            Repository = await OpenAsync();
        }
        catch (Exception e)
        {
            // OpenAsync sets SkipReason itself for a deliberate skip; anything else (Docker down, image pull failed)
            // is reported as a skip with the cause rather than failing every test of the class.
            SkipReason ??= $"{GetType().Name}: {e.GetType().Name}: {e.Message}";
            return;
        }

        var contacts = TestData.ReadRows("contact-data-100.csv").Select(row => ContactMapper.FromRow(row)).ToList();
        await Repository.BulkInsertAsync(contacts, CancellationToken.None);
    }

    public abstract Task DisposeAsync();

    protected abstract Task<IContactRepository> OpenAsync();
}

/// <summary>The same assertions for every profile; concrete classes supply the fixture.</summary>
public abstract class ContactRepositoryContract<TFixture>
    where TFixture : RepositoryFixture
{
    protected ContactRepositoryContract(TFixture fixture)
    {
        Fixture = fixture;
    }

    protected TFixture Fixture { get; }

    protected IContactRepository Repository
    {
        get
        {
            Skip.If(Fixture.SkipReason != null, Fixture.SkipReason);
            return Fixture.Repository;
        }
    }

    [SkippableFact]
    public async Task QueryByCityMatchesTheGoldenResponse()
    {
        var rows = await Repository.QueryByCityAsync("Saint Louis", 50, CancellationToken.None);

        Assert.Equal(
            Json.Canonical(TestData.ReadText("api/get-contacts-saint-louis.json")),
            Json.Canonical(ContactsQuery.RowsToJson("Saint Louis", rows)));
    }

    [SkippableFact]
    public async Task QueryByCityIsCaseInsensitive()
    {
        var lower = await Repository.QueryByCityAsync("saint louis", 50, CancellationToken.None);
        var upper = await Repository.QueryByCityAsync("SAINT LOUIS", 50, CancellationToken.None);

        Assert.Equal(2, lower.Count);
        Assert.Equal(lower, upper);
    }

    [SkippableFact]
    public async Task QueryByCityHonoursTheLimit()
    {
        var rows = await Repository.QueryByCityAsync("Saint Louis", 1, CancellationToken.None);

        var only = Assert.Single(rows);
        Assert.Equal("Hugenin", only.LastName);
    }

    [SkippableFact]
    public async Task QueryByCityOrdersByLastNameFirstNameContactId()
    {
        var rows = await Repository.QueryByCityAsync("Saint Louis", 50, CancellationToken.None);

        Assert.Equal(["Hugenin", "Punton"], rows.Select(r => r.LastName).ToList());
    }

    [SkippableFact]
    public async Task QueryByUnknownCityIsEmpty()
    {
        var rows = await Repository.QueryByCityAsync("Atlantis", 50, CancellationToken.None);

        Assert.Empty(rows);
    }

    [SkippableFact]
    public async Task BulkInsertOfNothingIsANoOp()
    {
        await Repository.BulkInsertAsync([], CancellationToken.None);

        Assert.Equal(2, (await Repository.QueryByCityAsync("Saint Louis", 50, CancellationToken.None)).Count);
    }

    [SkippableFact]
    public async Task RowsKeepEmptyStringsAsEmptyStrings()
    {
        var rows = await Repository.QueryByCityAsync("Saint Louis", 50, CancellationToken.None);

        Assert.All(rows, r => Assert.NotNull(r.Phone));
    }
}
