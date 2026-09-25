using Contacts.Core.Batch;
using Contacts.Core.Csv;
using Contacts.Core.Model;
using Contacts.Core.Pipeline;
using Contacts.Core.Validation;
using Contacts.Tests.Support;

namespace Contacts.Tests.Golden;

/// <summary>"Create Error Record" against <c>testdata/expected/errors-100-with-errors.json</c>.</summary>
public sealed class ErrorRecordGoldenTests
{
    [Fact]
    public void InvalidEmailRowsProduceTheGoldenErrorRecords()
    {
        var rows = TestData.ReadRows("contact-data-100-with-errors.csv");

        var records = rows
            .Select((row, index) => new BatchRecord<Dictionary<string, string>>(row, index))
            .Where(record => !EmailValidation.IsEmail(record.Payload["email"]))
            .Select(record =>
            {
                record.Fail(new InvalidEmailException(EmailValidation.InvalidEmailMessage));
                return ErrorRecord.Create(record);
            })
            .ToList();

        Assert.Equal(2, records.Count);
        Assert.Equal(Json.Canonical(TestData.ReadText("expected/errors-100-with-errors.json")), Json.Canonical(records));
    }

    [Fact]
    public void RecordLinesAreTheRowsReserialisedWithoutHeader()
    {
        var rows = TestData.ReadRows("contact-data-100-with-errors.csv");
        var golden = TestData.ReadJson("expected/errors-100-with-errors.json").AsArray();

        // Rows 3 and 7 of the file (external_id 3 and 7) are the two invalid ones.
        Assert.Equal(golden[0]!["Record"]!.GetValue<string>(), CsvRows.ToLine(rows[2], Contact.Columns));
        Assert.Equal(golden[1]!["Record"]!.GetValue<string>(), CsvRows.ToLine(rows[6], Contact.Columns));
        Assert.All(golden, entry => Assert.Equal(EmailValidation.InvalidEmailMessage, entry!["Error"]!.GetValue<string>()));
    }

    [Fact]
    public void CsvLineQuotesSeparatorsQuotesAndNewlinesOnly()
    {
        var row = new Dictionary<string, string>
        {
            ["a"] = "plain",
            ["b"] = "has,comma",
            ["c"] = "has \"quote\"",
            ["d"] = "two\nlines",
            ["e"] = " spaced ",
        };

        Assert.Equal("plain,\"has,comma\",\"has \"\"quote\"\"\",\"two\nlines\", spaced ", CsvRows.ToLine(row, ["a", "b", "c", "d", "e"]));
    }

    [Fact]
    public void CsvReaderHandlesQuotedCellsAndMissingTrailingCells()
    {
        using var reader = new StringReader("h1,h2,h3\n\"x,y\",\"say \"\"hi\"\"\"\n1,2,3\n");

        var rows = CsvRows.Read(reader).ToList();

        Assert.Equal(2, rows.Count);
        Assert.Equal("x,y", rows[0]["h1"]);
        Assert.Equal("say \"hi\"", rows[0]["h2"]);
        Assert.Equal("", rows[0]["h3"]);
        Assert.Equal("3", rows[1]["h3"]);
    }

    [Fact]
    public void CsvReaderJoinsQuotedCellsSpanningLines()
    {
        using var reader = new StringReader("h1,h2\n\"first\nsecond\",x\n");

        var row = Assert.Single(CsvRows.Read(reader));

        Assert.Equal("first\nsecond", row["h1"]);
        Assert.Equal("x", row["h2"]);
    }

    [Fact]
    public void CsvReaderRejectsAnUnterminatedQuote()
    {
        using var reader = new StringReader("h1,h2\n\"never closed,x\n");

        var e = Assert.Throws<CsvFormatException>(() => CsvRows.Read(reader).ToList());

        Assert.Contains("line 2", e.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void CsvReaderSkipsBlankLines()
    {
        using var reader = new StringReader("h1\n\na\n\nb\n");

        Assert.Equal(["a", "b"], CsvRows.Read(reader).Select(r => r["h1"]).ToList());
    }

    [Fact]
    public void CsvReaderRejectsEmptyInput()
    {
        using var reader = new StringReader("");

        Assert.Throws<CsvFormatException>(() => CsvRows.Read(reader).ToList());
    }
}
