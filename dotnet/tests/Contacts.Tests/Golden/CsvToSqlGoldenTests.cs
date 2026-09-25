using System.Text.Json;
using Contacts.Core.Mapping;
using Contacts.Core.Model;
using Contacts.Tests.Support;

namespace Contacts.Tests.Golden;

/// <summary>"CSV to Java" + "CSV to SQL" against <c>testdata/expected/first-record.json</c>.</summary>
public sealed class CsvToSqlGoldenTests
{
    private static readonly JsonSerializerOptions SnakeCase = new() { PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower };

    [Fact]
    public void FirstRecordOfContactData100MatchesGolden()
    {
        var row = TestData.ReadRows("contact-data-100.csv")[0];

        var contact = ContactMapper.FromRow(row);

        Assert.Equal(
            Json.Canonical(TestData.ReadText("expected/first-record.json")),
            Json.Canonical(contact, SnakeCase));
    }

    [Fact]
    public void CsvReaderYieldsOneRowPerDataLineWithAllColumns()
    {
        var rows = TestData.ReadRows("contact-data-100.csv");

        Assert.Equal(100, rows.Count);
        Assert.All(rows, row => Assert.Equal(Contact.Columns.Order(StringComparer.Ordinal), row.Keys.Order(StringComparer.Ordinal)));
    }

    [Fact]
    public void EveryRowOfContactData100Maps()
    {
        var rows = TestData.ReadRows("contact-data-100.csv");

        var contacts = rows.Select(row => ContactMapper.FromRow(row)).ToList();

        Assert.Equal(100, contacts.Count);
        Assert.All(contacts, c => Assert.False(string.IsNullOrEmpty(c.ContactId)));
    }

    [Fact]
    public void EmptyCellsStayEmptyStringsNotNull()
    {
        // Deviation 6: the first record has an empty fax and salutation.
        var contact = ContactMapper.FromRow(TestData.ReadRows("contact-data-100.csv")[0]);

        Assert.Equal("", contact.Fax);
        Assert.Equal("", contact.Salutation);
    }

    [Fact]
    public void AsDateDropsTheTimePartOfTimestampColumnsByDefault()
    {
        // Deviation 4: created_date is a date in the CSV, last_modified_date a full instant; 'as Date' keeps the date.
        var contact = ContactMapper.FromRow(TestData.ReadRows("contact-data-100.csv")[0]);

        Assert.Equal("2007-09-26", contact.CreatedDate.ToString());
        Assert.Equal("2025-05-01", contact.LastModifiedDate.ToString());
        Assert.Equal("2025-05-01", contact.SystemModStamp.ToString());
        Assert.True(contact.LastModifiedDate!.Value.IsDateOnly);
        Assert.Equal(new DateOnly(2025, 5, 1), contact.LastModifiedDate.Value.Date);
    }

    [Fact]
    public void InstantPrecisionKeepsTheTimePartAndOnlyChangesTheTimestampColumns()
    {
        var row = TestData.ReadRows("contact-data-100.csv")[0];

        var contact = ContactMapper.FromRow(row, TimestampPrecision.Instant);

        Assert.Equal("2007-09-26", contact.CreatedDate.ToString()); // a plain date in the input stays a date
        Assert.Equal("2025-05-01T11:39:15.257Z", contact.LastModifiedDate.ToString());
        Assert.Equal("2025-05-01T11:39:15.257Z", contact.SystemModStamp.ToString());
        Assert.False(contact.LastModifiedDate!.Value.IsDateOnly);

        var golden = TestData.ReadJson("expected/first-record.json").AsObject();
        golden["last_modified_date"] = "2025-05-01T11:39:15.257Z";
        golden["system_mod_stamp"] = "2025-05-01T11:39:15.257Z";
        Assert.Equal(Json.Canonical(golden), Json.Canonical(contact, SnakeCase));
    }

    [Theory]
    [InlineData("2025-05-01", 2025, 5, 1)]
    [InlineData("2025-05-01T11:39:15.257Z", 2025, 5, 1)]
    [InlineData("2025-05-01T23:59:59+02:00", 2025, 5, 1)] // the date as written, no zone conversion
    [InlineData("2025-05-01T00:30:00-05:00", 2025, 5, 1)]
    public void AsDateTakesTheCalendarDateAsWritten(string value, int year, int month, int day) =>
        Assert.Equal(new DateOnly(year, month, day), Coerce.ToDate(value, "f"));

    [Theory]
    [InlineData("2025-05-01", TimestampPrecision.Date, "2025-05-01")]
    [InlineData("2025-05-01", TimestampPrecision.Instant, "2025-05-01")]
    [InlineData("2025-05-01T23:59:59+02:00", TimestampPrecision.Date, "2025-05-01")]
    [InlineData("2025-05-01T23:59:59+02:00", TimestampPrecision.Instant, "2025-05-01T21:59:59.000Z")]
    [InlineData("2025-05-01T11:39:15", TimestampPrecision.Instant, "2025-05-01T11:39:15.000Z")] // no offset → UTC
    [InlineData("2025-05-01T11:39:15.2571234Z", TimestampPrecision.Instant, "2025-05-01T11:39:15.257Z")]
    public void ToTimestampFollowsThePrecision(string value, TimestampPrecision precision, string expected) =>
        Assert.Equal(expected, Coerce.ToTimestamp(value, "f", precision).ToString());

    [Fact]
    public void TimestampRoundTripsThroughJson()
    {
        foreach (var text in new[] { "2007-09-26", "2025-05-01T11:39:15.257Z" })
        {
            var parsed = Timestamp.Parse(text, TimestampPrecision.Instant);

            var json = JsonSerializer.Serialize(parsed);

            Assert.Equal($"\"{text}\"", json);
            Assert.Equal(parsed, JsonSerializer.Deserialize<Timestamp>(json));
        }
    }

    [Theory]
    [InlineData("abc", "Number")]
    [InlineData("1.5", "Number")]
    [InlineData("99999999999", "Number")]
    public void AsNumberRejectsNonIntegers(string value, string targetType)
    {
        var e = Assert.Throws<CoercionException>(() => Coerce.ToInt(value, "active_tracker_count"));

        Assert.Equal("active_tracker_count", e.Field);
        Assert.Equal(value, e.Value);
        Assert.Equal(targetType, e.TargetType);
        Assert.Equal($"Cannot coerce String ({value}) to {targetType}", e.Message); // DataWeave's own wording
    }

    [Theory]
    [InlineData("yes")]
    [InlineData("1")]
    public void AsBooleanRejectsAnythingButTrueFalse(string value) =>
        Assert.Throws<CoercionException>(() => Coerce.ToBool(value, "is_deleted"));

    [Theory]
    [InlineData("TRUE", true)]
    [InlineData("False", false)]
    public void AsBooleanIsCaseInsensitive(string value, bool expected) =>
        Assert.Equal(expected, Coerce.ToBool(value, "is_deleted"));

    [Fact]
    public void AsDateRejectsNonIsoDates()
    {
        var e = Assert.Throws<CoercionException>(() => Coerce.ToDate("12/18/1976", "birthdate"));

        Assert.IsType<FormatException>(e.InnerException);
    }

    [Fact]
    public void EmptyCellsCoerceToNull()
    {
        Assert.Null(Coerce.ToInt("", "f"));
        Assert.Null(Coerce.ToBool("", "f"));
        Assert.Null(Coerce.ToDate("", "f"));
        Assert.Null(Coerce.ToTimestamp("", "f", TimestampPrecision.Instant));
        Assert.Null(Coerce.ToInt(null, "f"));
    }

    [Fact]
    public void AsDateRejectsNonIsoDatesWithDataWeavesMessage()
    {
        var e = Assert.Throws<CoercionException>(() => Coerce.ToDate("May 1st", "created_date"));

        Assert.Equal("Cannot coerce String (May 1st) to Date", e.Message);
    }

    [Fact]
    public void MappingFailureNamesTheField()
    {
        var row = new Dictionary<string, string>(TestData.ReadRows("contact-data-100.csv")[0])
        {
            ["may_edit"] = "maybe",
        };

        var e = Assert.Throws<CoercionException>(() => ContactMapper.FromRow(row));

        Assert.Equal("may_edit", e.Field);
    }
}
