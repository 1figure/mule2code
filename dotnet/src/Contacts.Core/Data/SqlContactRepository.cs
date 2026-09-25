using System.Data.Common;
using System.Globalization;
using Contacts.Core.Model;
using Contacts.Core.Settings;
using Dapper;
using Microsoft.Data.Sqlite;
using Npgsql;

namespace Contacts.Core.Data;

/// <summary>
/// <see cref="IContactRepository"/> on Microsoft.Data.Sqlite or Npgsql through Dapper; replaces
/// <c>&lt;db:config&gt;&lt;db:generic-connection url= driverClassName=&gt;</c>. A connection is opened per operation.
/// </summary>
public sealed class SqlContactRepository : IContactRepository
{
    // The upstream INSERT with ':name' parameters spelled '@name', which both providers accept.
    private const string InsertSql =
        "INSERT INTO contacts(account_id, active_tracker_count, assistant_name, assistant_phone, birthdate, can_allow_portal_self_reg, contact_id, contact_name, created_by_id, created_date, currency_iso_code, department, do_not_call, email, external_id, fax, first_name, has_opted_out_of_email, has_opted_out_of_fax, has_privacy_hold, home_phone, is_deleted, is_email_bounced, is_locked, is_person_account, is_priority_record, last_modified_by_id, last_modified_date, last_name, lead_source, mailing_city, mailing_country, mailing_postal_code, mailing_state, mailing_street, may_edit, middle_name, mobile_phone, other_city, other_country, other_phone, other_postal_code, other_state, other_street, owner_id, phone, photo_url, record_type_id, salutation, suffix, system_mod_stamp, title)\n" +
        "VALUES (@account_id, @active_tracker_count, @assistant_name, @assistant_phone, @birthdate, @can_allow_portal_self_reg, @contact_id, @contact_name, @created_by_id, @created_date, @currency_iso_code, @department, @do_not_call, @email, @external_id, @fax, @first_name, @has_opted_out_of_email, @has_opted_out_of_fax, @has_privacy_hold, @home_phone, @is_deleted, @is_email_bounced, @is_locked, @is_person_account, @is_priority_record, @last_modified_by_id, @last_modified_date, @last_name, @lead_source, @mailing_city, @mailing_country, @mailing_postal_code, @mailing_state, @mailing_street, @may_edit, @middle_name, @mobile_phone, @other_city, @other_country, @other_phone, @other_postal_code, @other_state, @other_street, @owner_id, @phone, @photo_url, @record_type_id, @salutation, @suffix, @system_mod_stamp, @title)";

    // The upstream SELECT; columns are aliased to the ContactRow property names so no global Dapper mapping is needed.
    private const string SelectByCitySql =
        "SELECT contact_id AS ContactId, first_name AS FirstName, last_name AS LastName, email AS Email, phone AS Phone,\n" +
        "       title AS Title, department AS Department, mailing_street AS MailingStreet, mailing_city AS MailingCity,\n" +
        "       mailing_state AS MailingState, mailing_postal_code AS MailingPostalCode, mailing_country AS MailingCountry\n" +
        "FROM contacts\n" +
        "WHERE LOWER(mailing_city) = LOWER(@city)\n" +
        "ORDER BY last_name, first_name, contact_id\n" +
        "LIMIT @limit";

    private readonly DbProfile profile;
    private readonly string connectionString;

    static SqlContactRepository()
    {
        // Dapper has no built-in mapping for DateOnly parameters (its handlers are process-wide by design);
        // Npgsql binds it as a native DATE.
        SqlMapper.AddTypeHandler(new DateOnlyHandler());
    }

    private sealed class DateOnlyHandler : SqlMapper.TypeHandler<DateOnly>
    {
        public override void SetValue(System.Data.IDbDataParameter parameter, DateOnly value)
        {
            parameter.DbType = System.Data.DbType.Date;
            parameter.Value = value;
        }

        public override DateOnly Parse(object value) => value switch
        {
            DateOnly d => d,
            DateTime dt => DateOnly.FromDateTime(dt),
            string s => DateOnly.Parse(s, CultureInfo.InvariantCulture),
            _ => throw new InvalidCastException($"Cannot convert {value.GetType()} to DateOnly"),
        };
    }

    private SqlContactRepository(DbProfile profile, string connectionString)
    {
        this.profile = profile;
        this.connectionString = connectionString;
    }

    /// <summary>The profile this repository binds values for.</summary>
    public DbProfile Profile => profile;

    /// <summary>
    /// Creates the repository for <paramref name="profile"/>, verifies the connection and applies the schema:
    /// <c>docker/sqlite/contacts.sqlite.sql</c> on every start (it is idempotent), <c>docker/init/01-contacts.sql</c>
    /// only when the <c>contacts</c> table does not exist yet.
    /// </summary>
    /// <exception cref="RepositoryException">The database cannot be reached or the schema failed.</exception>
    public static async Task<SqlContactRepository> OpenAsync(DbProfile profile, string connectionString, CancellationToken cancellationToken)
    {
        ArgumentException.ThrowIfNullOrEmpty(connectionString);
        var repository = new SqlContactRepository(profile, connectionString);
        await repository.ApplySchemaAsync(cancellationToken).ConfigureAwait(false);
        return repository;
    }

    /// <inheritdoc />
    public async Task BulkInsertAsync(IReadOnlyList<Contact> contacts, CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(contacts);
        if (contacts.Count == 0)
        {
            return;
        }

        try
        {
            await using var connection = await OpenConnectionAsync(cancellationToken).ConfigureAwait(false);
            await using var transaction = await connection.BeginTransactionAsync(cancellationToken).ConfigureAwait(false);
            var parameters = contacts.Select(BindParameters).ToList();
            await connection.ExecuteAsync(new CommandDefinition(InsertSql, parameters, transaction, cancellationToken: cancellationToken)).ConfigureAwait(false);
            await transaction.CommitAsync(cancellationToken).ConfigureAwait(false);
        }
        catch (DbException e)
        {
            throw new RepositoryException($"Bulk insert of {contacts.Count} contacts failed: {e.Message}", e);
        }
    }

    /// <inheritdoc />
    public async Task<IReadOnlyList<ContactRow>> QueryByCityAsync(string city, int limit, CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(city);
        try
        {
            await using var connection = await OpenConnectionAsync(cancellationToken).ConfigureAwait(false);
            var rows = await connection.QueryAsync<ContactRow>(
                new CommandDefinition(SelectByCitySql, new { city, limit }, cancellationToken: cancellationToken)).ConfigureAwait(false);
            return rows.ToList();
        }
        catch (DbException e)
        {
            throw new RepositoryException($"Select contacts by city failed: {e.Message}", e);
        }
    }

    /// <summary>Binds one contact for the <c>INSERT</c>, as MAPPING.md deviation 7 prescribes for each profile.</summary>
    /// <remarks>Public so a test can assert the exact SQLite representations.</remarks>
    public DynamicParameters BindParameters(Contact contact)
    {
        ArgumentNullException.ThrowIfNull(contact);
        var p = new DynamicParameters();
        p.Add("account_id", contact.AccountId);
        p.Add("active_tracker_count", contact.ActiveTrackerCount);
        p.Add("assistant_name", contact.AssistantName);
        p.Add("assistant_phone", contact.AssistantPhone);
        p.Add("birthdate", Date(contact.Birthdate));
        p.Add("can_allow_portal_self_reg", Bool(contact.CanAllowPortalSelfReg));
        p.Add("contact_id", contact.ContactId);
        p.Add("contact_name", contact.ContactName);
        p.Add("created_by_id", contact.CreatedById);
        p.Add("created_date", Timestamp(contact.CreatedDate));
        p.Add("currency_iso_code", contact.CurrencyIsoCode);
        p.Add("department", contact.Department);
        p.Add("do_not_call", Bool(contact.DoNotCall));
        p.Add("email", contact.Email);
        p.Add("external_id", contact.ExternalId);
        p.Add("fax", contact.Fax);
        p.Add("first_name", contact.FirstName);
        p.Add("has_opted_out_of_email", Bool(contact.HasOptedOutOfEmail));
        p.Add("has_opted_out_of_fax", Bool(contact.HasOptedOutOfFax));
        p.Add("has_privacy_hold", Bool(contact.HasPrivacyHold));
        p.Add("home_phone", contact.HomePhone);
        p.Add("is_deleted", Bool(contact.IsDeleted));
        p.Add("is_email_bounced", Bool(contact.IsEmailBounced));
        p.Add("is_locked", Bool(contact.IsLocked));
        p.Add("is_person_account", Bool(contact.IsPersonAccount));
        p.Add("is_priority_record", Bool(contact.IsPriorityRecord));
        p.Add("last_modified_by_id", contact.LastModifiedById);
        p.Add("last_modified_date", Timestamp(contact.LastModifiedDate));
        p.Add("last_name", contact.LastName);
        p.Add("lead_source", contact.LeadSource);
        p.Add("mailing_city", contact.MailingCity);
        p.Add("mailing_country", contact.MailingCountry);
        p.Add("mailing_postal_code", contact.MailingPostalCode);
        p.Add("mailing_state", contact.MailingState);
        p.Add("mailing_street", contact.MailingStreet);
        p.Add("may_edit", Bool(contact.MayEdit));
        p.Add("middle_name", contact.MiddleName);
        p.Add("mobile_phone", contact.MobilePhone);
        p.Add("other_city", contact.OtherCity);
        p.Add("other_country", contact.OtherCountry);
        p.Add("other_phone", contact.OtherPhone);
        p.Add("other_postal_code", contact.OtherPostalCode);
        p.Add("other_state", contact.OtherState);
        p.Add("other_street", contact.OtherStreet);
        p.Add("owner_id", contact.OwnerId);
        p.Add("phone", contact.Phone);
        p.Add("photo_url", contact.PhotoUrl);
        p.Add("record_type_id", contact.RecordTypeId);
        p.Add("salutation", contact.Salutation);
        p.Add("suffix", contact.Suffix);
        p.Add("system_mod_stamp", Timestamp(contact.SystemModStamp));
        p.Add("title", contact.Title);
        return p;
    }

    /// <summary>SQLite has no BOOLEAN: <c>0</c>/<c>1</c>. Postgres keeps the native type.</summary>
    public object? Bool(bool? value) => value is null
        ? null
        : profile == DbProfile.Sqlite ? (value.Value ? 1L : 0L) : value.Value;

    /// <summary>SQLite has no DATE: <c>YYYY-MM-DD</c> text. Postgres keeps the native type.</summary>
    public object? Date(DateOnly? value) => value is null
        ? null
        : profile == DbProfile.Sqlite ? value.Value.ToString("yyyy-MM-dd", CultureInfo.InvariantCulture) : value.Value;

    /// <summary>
    /// The <c>TIMESTAMP WITH TIME ZONE</c> columns (deviation 4). SQLite stores the value's text — <c>YYYY-MM-DD</c>
    /// for a date-only value, ISO-8601 UTC with milliseconds otherwise; Postgres gets the UTC instant (midnight for a
    /// date-only value), bound explicitly so it does not depend on the session time zone the way a
    /// <c>date → timestamptz</c> cast would.
    /// </summary>
    public object? Timestamp(Timestamp? value) => value is null
        ? null
        : profile == DbProfile.Sqlite ? value.Value.ToString() : value.Value.Value;

    private async Task<DbConnection> OpenConnectionAsync(CancellationToken cancellationToken)
    {
        DbConnection connection = profile == DbProfile.Sqlite
            ? new SqliteConnection(connectionString)
            : new NpgsqlConnection(connectionString);
        try
        {
            await connection.OpenAsync(cancellationToken).ConfigureAwait(false);
            return connection;
        }
        catch
        {
            await connection.DisposeAsync().ConfigureAwait(false);
            throw;
        }
    }

    private async Task ApplySchemaAsync(CancellationToken cancellationToken)
    {
        try
        {
            if (profile == DbProfile.Sqlite)
            {
                var dataSource = new SqliteConnectionStringBuilder(connectionString).DataSource;
                if (Path.GetDirectoryName(dataSource) is { Length: > 0 } directory)
                {
                    Directory.CreateDirectory(directory);
                }
            }

            await using var connection = await OpenConnectionAsync(cancellationToken).ConfigureAwait(false);
            if (profile == DbProfile.Postgres)
            {
                var exists = await connection.ExecuteScalarAsync<bool>(
                    new CommandDefinition("SELECT to_regclass('public.contacts') IS NOT NULL", cancellationToken: cancellationToken)).ConfigureAwait(false);
                if (exists)
                {
                    return;
                }
            }

            var schemaPath = profile == DbProfile.Sqlite ? Resources.SqliteSchema : Resources.PostgresSchema;
            var schema = await File.ReadAllTextAsync(schemaPath, cancellationToken).ConfigureAwait(false);
            await connection.ExecuteAsync(new CommandDefinition(schema, cancellationToken: cancellationToken)).ConfigureAwait(false);
        }
        catch (Exception e) when (e is DbException or IOException)
        {
            // IOException covers a missing schema file (FileNotFoundException) as well as an unreadable database directory.
            throw new RepositoryException($"Cannot open the {profile} database or apply its schema: {e.Message}", e);
        }
    }
}
