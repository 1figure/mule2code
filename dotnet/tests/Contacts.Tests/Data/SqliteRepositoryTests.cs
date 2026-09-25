using Contacts.Core.Data;
using Contacts.Core.Model;
using Contacts.Core.Settings;
using Contacts.Tests.Support;
using Microsoft.Data.Sqlite;

namespace Contacts.Tests.Data;

public sealed class SqliteRepositoryFixture : RepositoryFixture
{
    private readonly TempDir dir = new();

    public string ConnectionString => $"Data Source={Path.Combine(dir.Path, "nested", "contacts.db")}";

    public override Task DisposeAsync()
    {
        dir.Dispose();
        return Task.CompletedTask;
    }

    protected override async Task<IContactRepository> OpenAsync() =>
        await SqlContactRepository.OpenAsync(DbProfile.Sqlite, ConnectionString, CancellationToken.None);
}

/// <summary>The repository contract on SQLite, plus the deviation-7 storage representations.</summary>
public sealed class SqliteRepositoryTests : ContactRepositoryContract<SqliteRepositoryFixture>, IClassFixture<SqliteRepositoryFixture>
{
    public SqliteRepositoryTests(SqliteRepositoryFixture fixture) : base(fixture)
    {
    }

    [Fact]
    public async Task SchemaIsAppliedAndDirectoryCreatedOnOpen()
    {
        await using var connection = new SqliteConnection(Fixture.ConnectionString);
        await connection.OpenAsync();
        await using var command = connection.CreateCommand();
        command.CommandText = "SELECT COUNT(*) FROM contacts";

        Assert.Equal(100L, await command.ExecuteScalarAsync());
    }

    [Fact]
    public async Task OpenIsIdempotent()
    {
        var again = await SqlContactRepository.OpenAsync(DbProfile.Sqlite, Fixture.ConnectionString, CancellationToken.None);

        Assert.Equal(2, (await again.QueryByCityAsync("Saint Louis", 50, CancellationToken.None)).Count);
    }

    [Fact]
    public void BindsBooleansDatesAndTimestampsAsText()
    {
        var repository = (SqlContactRepository)Fixture.Repository;

        Assert.Equal(1L, repository.Bool(true));
        Assert.Equal(0L, repository.Bool(false));
        Assert.Null(repository.Bool(null));
        Assert.Equal("1976-12-18", repository.Date(new DateOnly(1976, 12, 18)));
        Assert.Equal("2025-05-01", repository.Timestamp(Timestamp.Parse("2025-05-01T13:39:15.257+02:00", TimestampPrecision.Date)));
        Assert.Equal("2025-05-01T11:39:15.257Z", repository.Timestamp(Timestamp.Parse("2025-05-01T13:39:15.257+02:00", TimestampPrecision.Instant)));
        Assert.Null(repository.Timestamp(null));
    }

    [Fact]
    public async Task StoredRowUsesTheSharedRepresentations()
    {
        await using var connection = new SqliteConnection(Fixture.ConnectionString);
        await connection.OpenAsync();
        await using var command = connection.CreateCommand();
        command.CommandText = "SELECT typeof(may_edit), may_edit, typeof(birthdate), birthdate, created_date, system_mod_stamp FROM contacts WHERE external_id = '1'";
        await using var reader = await command.ExecuteReaderAsync();
        Assert.True(await reader.ReadAsync());

        Assert.Equal("integer", reader.GetString(0));
        Assert.Equal(1L, reader.GetInt64(1));
        Assert.Equal("text", reader.GetString(2));
        Assert.Equal("1976-12-18", reader.GetString(3));
        Assert.Equal("2007-09-26", reader.GetString(4));
        Assert.Equal("2025-05-01", reader.GetString(5));
    }

    [Fact]
    public async Task UnreachableDatabaseIsARepositoryException()
    {
        var path = Path.Combine(Path.GetTempPath(), "contacts-tests-missing-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(path);
        try
        {
            var e = await Assert.ThrowsAsync<RepositoryException>(() =>
                SqlContactRepository.OpenAsync(DbProfile.Sqlite, $"Data Source={path};Mode=ReadOnly", CancellationToken.None));

            Assert.IsAssignableFrom<System.Data.Common.DbException>(e.InnerException);
        }
        finally
        {
            SqliteConnection.ClearAllPools();
            Directory.Delete(path, recursive: true);
        }
    }
}
