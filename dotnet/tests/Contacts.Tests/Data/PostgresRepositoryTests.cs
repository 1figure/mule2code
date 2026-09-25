using Contacts.Core.Data;
using Contacts.Core.Model;
using Contacts.Core.Settings;
using Npgsql;
using Testcontainers.PostgreSql;

namespace Contacts.Tests.Data;

/// <summary>
/// A throw-away PostgreSQL 16 through Testcontainers. Opt-in: runs only with <c>RUN_POSTGRES=1</c>, and skips when
/// Docker is absent.
/// </summary>
public sealed class PostgresRepositoryFixture : RepositoryFixture
{
    private PostgreSqlContainer? container;

    public string ConnectionString => container?.GetConnectionString() ?? throw new InvalidOperationException("container not started");

    public override async Task DisposeAsync()
    {
        if (container != null)
        {
            await container.DisposeAsync();
        }
    }

    protected override async Task<IContactRepository> OpenAsync()
    {
        if (Environment.GetEnvironmentVariable("RUN_POSTGRES") is not ("1" or "true"))
        {
            SkipReason = "Postgres tests are opt-in: set RUN_POSTGRES=1 (needs Docker)";
            throw new InvalidOperationException(SkipReason);
        }

        if (string.IsNullOrEmpty(Environment.GetEnvironmentVariable("DOCKER_HOST")) && !File.Exists("/var/run/docker.sock"))
        {
            SkipReason = "Docker is not available";
            throw new InvalidOperationException(SkipReason);
        }

        container = new PostgreSqlBuilder().WithImage("postgres:16-alpine").Build();
        await container.StartAsync();
        return await SqlContactRepository.OpenAsync(DbProfile.Postgres, container.GetConnectionString(), CancellationToken.None);
    }
}

/// <summary>The repository contract on PostgreSQL, plus the native typing the profile keeps.</summary>
public sealed class PostgresRepositoryTests : ContactRepositoryContract<PostgresRepositoryFixture>, IClassFixture<PostgresRepositoryFixture>
{
    public PostgresRepositoryTests(PostgresRepositoryFixture fixture) : base(fixture)
    {
    }

    [SkippableFact]
    public async Task SchemaIsAppliedFromTheUpstreamDdl()
    {
        _ = Repository;
        await using var connection = new NpgsqlConnection(Fixture.ConnectionString);
        await connection.OpenAsync();
        await using var command = new NpgsqlCommand("SELECT COUNT(*) FROM public.contacts", connection);

        Assert.Equal(100L, await command.ExecuteScalarAsync());
    }

    [SkippableFact]
    public async Task OpenIsIdempotentWhenTheTableExists()
    {
        _ = Repository;

        var again = await SqlContactRepository.OpenAsync(DbProfile.Postgres, Fixture.ConnectionString, CancellationToken.None);

        Assert.Equal(2, (await again.QueryByCityAsync("Saint Louis", 50, CancellationToken.None)).Count);
    }

    [SkippableFact]
    public void BindsNativeTypes()
    {
        var repository = (SqlContactRepository)Repository;

        Assert.Equal(true, repository.Bool(true));
        Assert.Equal(new DateOnly(1976, 12, 18), repository.Date(new DateOnly(1976, 12, 18)));
        Assert.Equal(new DateTimeOffset(2025, 5, 1, 0, 0, 0, TimeSpan.Zero), repository.Timestamp(Timestamp.Parse("2025-05-01T13:39:15.257+02:00", TimestampPrecision.Date)));
        Assert.Equal(new DateTimeOffset(2025, 5, 1, 11, 39, 15, 257, TimeSpan.Zero), repository.Timestamp(Timestamp.Parse("2025-05-01T13:39:15.257+02:00", TimestampPrecision.Instant)));
    }

    [SkippableFact]
    public async Task StoredRowKeepsNativeTypes()
    {
        _ = Repository;
        await using var connection = new NpgsqlConnection(Fixture.ConnectionString);
        await connection.OpenAsync();
        await using var command = new NpgsqlCommand("SELECT may_edit, do_not_call, birthdate, created_date, last_modified_date, active_tracker_count, fax FROM contacts WHERE external_id = '1'", connection);
        await using var reader = await command.ExecuteReaderAsync();
        Assert.True(await reader.ReadAsync());

        Assert.True(reader.GetBoolean(0));
        Assert.False(reader.GetBoolean(1));
        Assert.Equal(new DateOnly(1976, 12, 18), reader.GetFieldValue<DateOnly>(2));
        Assert.Equal(new DateTime(2007, 9, 26, 0, 0, 0, DateTimeKind.Utc), reader.GetFieldValue<DateTime>(3));
        Assert.Equal(new DateTime(2025, 5, 1, 0, 0, 0, DateTimeKind.Utc), reader.GetFieldValue<DateTime>(4)); // 'as Date' (deviation 4)
        Assert.Equal(0, reader.GetInt32(5));
        Assert.Equal("", reader.GetString(6));
    }

    [SkippableFact]
    public async Task UnreachableDatabaseIsARepositoryException()
    {
        _ = Repository;

        var e = await Assert.ThrowsAsync<RepositoryException>(() =>
            SqlContactRepository.OpenAsync(DbProfile.Postgres, "Host=127.0.0.1;Port=1;Database=x;Username=x;Password=x;Timeout=2", CancellationToken.None));

        Assert.IsAssignableFrom<System.Data.Common.DbException>(e.InnerException);
    }
}
