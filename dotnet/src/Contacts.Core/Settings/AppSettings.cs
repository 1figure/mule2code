using Contacts.Core.Model;

namespace Contacts.Core.Settings;

/// <summary>The database profile, selected by <c>DB_PROFILE</c>.</summary>
public enum DbProfile
{
    /// <summary>SQLite file database; the schema is applied on start.</summary>
    Sqlite,

    /// <summary>PostgreSQL, the profile the original Mule application was written against.</summary>
    Postgres,
}

/// <summary>
/// Configuration of both applications, read from environment variables.
/// Replaces <c>&lt;global-property name="env"&gt;</c> + <c>properties/mule-props-${env}.yaml</c>.
/// </summary>
public sealed record AppSettings
{
    /// <summary><c>DB_PROFILE</c>: <c>sqlite</c> (default) or <c>postgres</c>.</summary>
    public DbProfile DbProfile { get; init; } = DbProfile.Sqlite;

    /// <summary><c>SQLITE_PATH</c>: path of the SQLite database file (default <c>&lt;DATA_DIR&gt;/contacts.db</c>).</summary>
    public string SqlitePath { get; init; } = "";

    /// <summary><c>POSTGRES_CONNECTION_STRING</c> (default matches <c>docker/docker-compose.yml</c>).</summary>
    public string PostgresConnectionString { get; init; } = "Host=localhost;Port=5432;Database=contacts;Username=contacts;Password=contacts";

    /// <summary><c>DATA_DIR</c>: root of <c>inbox/</c>, <c>processed/</c>, <c>failed/</c>, <c>reports/</c> (default <c>data</c>).</summary>
    public string DataDir { get; init; } = "data";

    /// <summary><c>BATCH_BLOCK_SIZE</c> → <c>${batch.job.block_size}</c> (default 10000).</summary>
    public int BatchBlockSize { get; init; } = 10000;

    /// <summary><c>BATCH_AGGREGATOR_SIZE</c> → <c>${batch.aggregator.main.size}</c> (default 1000).</summary>
    public int BatchAggregatorSize { get; init; } = 1000;

    /// <summary><c>POLL_SECONDS</c>: polling interval of the inbox watcher (the fixed-frequency scheduler; default 10).</summary>
    public int PollSeconds { get; init; } = 10;

    /// <summary><c>HTTP_URL</c>: Kestrel listen URL (default <c>http://0.0.0.0:8082</c>, as <c>${http.host}</c>/<c>${http.port}</c>).</summary>
    public string HttpUrl { get; init; } = "http://0.0.0.0:8082";

    /// <summary><c>ZIPPOPOTAM_BASE_URL</c> → <c>&lt;http:request-config&gt;</c> (default <c>https://api.zippopotam.us</c>).</summary>
    public string ZippopotamBaseUrl { get; init; } = "https://api.zippopotam.us";

    /// <summary><c>NORMALIZE_MAX_CONTACTS</c> → <c>${normalize.max_contacts}</c> (default 50).</summary>
    public int NormalizeMaxContacts { get; init; } = 50;

    /// <summary>
    /// <c>TIMESTAMP_PRECISION</c>: <c>date</c> (default, Mule's <c>as Date</c> on the three timestamp columns) or
    /// <c>instant</c> (keep the full timestamp) — MAPPING.md deviation 4.
    /// </summary>
    public TimestampPrecision TimestampPrecision { get; init; } = TimestampPrecision.Date;

    /// <summary>The connection string of the selected profile.</summary>
    public string ConnectionString => DbProfile switch
    {
        DbProfile.Postgres => PostgresConnectionString,
        _ => $"Data Source={SqlitePath}",
    };

    /// <summary>Directory the batch listener polls (<c>${sftp.new_dir}</c>).</summary>
    public string InboxDir => Path.Combine(DataDir, "inbox");

    /// <summary>Directory successful files and error files are written to (<c>${sftp.processed_dir}</c>).</summary>
    public string ProcessedDir => Path.Combine(DataDir, "processed");

    /// <summary>Directory failed files are moved to (<c>${sftp.failed_dir}</c>).</summary>
    public string FailedDir => Path.Combine(DataDir, "failed");

    /// <summary>Directory the report file sink writes to (replaces the e-mail).</summary>
    public string ReportsDir => Path.Combine(DataDir, "reports");

    /// <summary>Reads the settings from the process environment, falling back to the defaults above.</summary>
    /// <exception cref="SettingsException">A variable has a value that cannot be parsed.</exception>
    public static AppSettings FromEnvironment() =>
        FromLookup(name => Environment.GetEnvironmentVariable(name));

    /// <summary>Reads the settings through <paramref name="lookup"/>; <c>null</c> or empty means "not set".</summary>
    /// <exception cref="SettingsException">A variable has a value that cannot be parsed.</exception>
    public static AppSettings FromLookup(Func<string, string?> lookup)
    {
        var defaults = new AppSettings();
        string Text(string name, string fallback) => lookup(name) is { Length: > 0 } value ? value : fallback;

        var dataDir = Text("DATA_DIR", defaults.DataDir);
        return new AppSettings
        {
            DbProfile = ParseProfile(lookup("DB_PROFILE")),
            SqlitePath = Text("SQLITE_PATH", Path.Combine(dataDir, "contacts.db")),
            PostgresConnectionString = Text("POSTGRES_CONNECTION_STRING", defaults.PostgresConnectionString),
            DataDir = dataDir,
            BatchBlockSize = ParseInt("BATCH_BLOCK_SIZE", lookup, defaults.BatchBlockSize),
            BatchAggregatorSize = ParseInt("BATCH_AGGREGATOR_SIZE", lookup, defaults.BatchAggregatorSize),
            PollSeconds = ParseInt("POLL_SECONDS", lookup, defaults.PollSeconds),
            HttpUrl = Text("HTTP_URL", defaults.HttpUrl),
            ZippopotamBaseUrl = Text("ZIPPOPOTAM_BASE_URL", defaults.ZippopotamBaseUrl),
            NormalizeMaxContacts = ParseInt("NORMALIZE_MAX_CONTACTS", lookup, defaults.NormalizeMaxContacts),
            TimestampPrecision = ParsePrecision(lookup("TIMESTAMP_PRECISION")),
        };
    }

    private static DbProfile ParseProfile(string? value) => value?.Trim().ToLowerInvariant() switch
    {
        null or "" or "sqlite" => DbProfile.Sqlite,
        "postgres" or "postgresql" => DbProfile.Postgres,
        _ => throw new SettingsException($"DB_PROFILE must be 'sqlite' or 'postgres', got '{value}'"),
    };

    private static TimestampPrecision ParsePrecision(string? value) => value?.Trim().ToLowerInvariant() switch
    {
        null or "" or "date" => TimestampPrecision.Date,
        "instant" => TimestampPrecision.Instant,
        _ => throw new SettingsException($"TIMESTAMP_PRECISION must be 'date' or 'instant', got '{value}'"),
    };

    private static int ParseInt(string name, Func<string, string?> lookup, int fallback)
    {
        var raw = lookup(name);
        if (string.IsNullOrEmpty(raw))
        {
            return fallback;
        }

        try
        {
            return int.Parse(raw, System.Globalization.CultureInfo.InvariantCulture);
        }
        catch (FormatException e)
        {
            throw new SettingsException($"{name} must be an integer, got '{raw}'", e);
        }
        catch (OverflowException e)
        {
            throw new SettingsException($"{name} must be an integer, got '{raw}'", e);
        }
    }
}

/// <summary>Thrown when an environment variable holds a value that cannot be used.</summary>
public sealed class SettingsException : Exception
{
    /// <summary>Creates the exception with a message.</summary>
    public SettingsException(string message) : base(message)
    {
    }

    /// <summary>Creates the exception with a message and the parse failure that caused it.</summary>
    public SettingsException(string message, Exception inner) : base(message, inner)
    {
    }
}
