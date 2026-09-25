using Contacts.Core.Model;
using Contacts.Core.Settings;

namespace Contacts.Tests.Settings;

/// <summary>The environment-variable names of MAPPING.md and their defaults.</summary>
[Collection("process-environment")]
public sealed class AppSettingsTests
{
    private static AppSettings From(Dictionary<string, string?> env) => AppSettings.FromLookup(name => env.GetValueOrDefault(name));

    [Fact]
    public void DefaultsMatchTheSqliteQuickStart()
    {
        var s = From([]);

        Assert.Equal(DbProfile.Sqlite, s.DbProfile);
        Assert.Equal(Path.Combine("data", "contacts.db"), s.SqlitePath);
        Assert.Equal("Data Source=" + Path.Combine("data", "contacts.db"), s.ConnectionString);
        Assert.Equal(Path.Combine("data", "inbox"), s.InboxDir);
        Assert.Equal(Path.Combine("data", "processed"), s.ProcessedDir);
        Assert.Equal(Path.Combine("data", "failed"), s.FailedDir);
        Assert.Equal(Path.Combine("data", "reports"), s.ReportsDir);
        Assert.Equal(10000, s.BatchBlockSize);
        Assert.Equal(1000, s.BatchAggregatorSize);
        Assert.Equal(10, s.PollSeconds);
        Assert.Equal("http://0.0.0.0:8082", s.HttpUrl);
        Assert.Equal("https://api.zippopotam.us", s.ZippopotamBaseUrl);
        Assert.Equal(50, s.NormalizeMaxContacts);
        Assert.Equal(TimestampPrecision.Date, s.TimestampPrecision);
    }

    [Theory]
    [InlineData("instant", TimestampPrecision.Instant)]
    [InlineData("Instant", TimestampPrecision.Instant)]
    [InlineData("date", TimestampPrecision.Date)]
    [InlineData("", TimestampPrecision.Date)]
    public void TimestampPrecisionIsReadCaseInsensitively(string value, TimestampPrecision expected) =>
        Assert.Equal(expected, From(new Dictionary<string, string?> { ["TIMESTAMP_PRECISION"] = value }).TimestampPrecision);

    [Fact]
    public void EveryVariableFromMappingMdIsRead()
    {
        var s = From(new Dictionary<string, string?>
        {
            ["DB_PROFILE"] = "postgres",
            ["SQLITE_PATH"] = "/x/y.db",
            ["POSTGRES_CONNECTION_STRING"] = "Host=h;Database=d",
            ["DATA_DIR"] = "/data",
            ["BATCH_BLOCK_SIZE"] = "7",
            ["BATCH_AGGREGATOR_SIZE"] = "3",
            ["POLL_SECONDS"] = "1",
            ["HTTP_URL"] = "http://localhost:1234",
            ["ZIPPOPOTAM_BASE_URL"] = "http://stub",
            ["NORMALIZE_MAX_CONTACTS"] = "2",
        });

        Assert.Equal(DbProfile.Postgres, s.DbProfile);
        Assert.Equal("/x/y.db", s.SqlitePath);
        Assert.Equal("Host=h;Database=d", s.ConnectionString);
        Assert.Equal(Path.Combine("/data", "inbox"), s.InboxDir);
        Assert.Equal(7, s.BatchBlockSize);
        Assert.Equal(3, s.BatchAggregatorSize);
        Assert.Equal(1, s.PollSeconds);
        Assert.Equal("http://localhost:1234", s.HttpUrl);
        Assert.Equal("http://stub", s.ZippopotamBaseUrl);
        Assert.Equal(2, s.NormalizeMaxContacts);
    }

    [Fact]
    public void EmptyValuesMeanUnsetForEveryVariable()
    {
        var empty = new Dictionary<string, string?>
        {
            ["DB_PROFILE"] = "",
            ["SQLITE_PATH"] = "",
            ["POSTGRES_CONNECTION_STRING"] = "",
            ["DATA_DIR"] = "",
            ["BATCH_BLOCK_SIZE"] = "",
            ["HTTP_URL"] = "",
            ["ZIPPOPOTAM_BASE_URL"] = "",
            ["NORMALIZE_MAX_CONTACTS"] = "",
        };

        Assert.Equal(From([]), From(empty));
    }

    [Fact]
    public void SqlitePathFollowsDataDirUnlessSet()
    {
        Assert.Equal(Path.Combine("/d", "contacts.db"), From(new Dictionary<string, string?> { ["DATA_DIR"] = "/d" }).SqlitePath);
        Assert.Equal("/elsewhere.db", From(new Dictionary<string, string?> { ["DATA_DIR"] = "/d", ["SQLITE_PATH"] = "/elsewhere.db" }).SqlitePath);
    }

    [Theory]
    [InlineData("DB_PROFILE", "oracle")]
    [InlineData("BATCH_BLOCK_SIZE", "many")]
    [InlineData("POLL_SECONDS", "99999999999")]
    [InlineData("TIMESTAMP_PRECISION", "millis")]
    public void UnparsableValuesAreSettingsExceptions(string name, string value)
    {
        var e = Assert.Throws<SettingsException>(() => From(new Dictionary<string, string?> { [name] = value }));

        Assert.Contains(name, e.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void FromEnvironmentReadsTheProcessEnvironment()
    {
        var s = AppSettings.FromEnvironment();

        Assert.NotNull(s.HttpUrl);
    }
}
