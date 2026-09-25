using System.Text;

namespace Contacts.Core.Csv;

/// <summary>
/// The CSV reader and writer that the DataWeave <c>application/csv</c> format provided.
/// Reads a file with a header row into lazily produced dictionaries and serialises one row back into a line.
/// </summary>
public static class CsvRows
{
    private const char Separator = ',';
    private const char Quote = '"';

    /// <summary>
    /// <c>&lt;ee:transform&gt;</c> "CSV to Java": <c>payload as Iterator</c> on an <c>application/csv; header=true</c> payload.
    /// Yields one dictionary per data row keyed by the header names; missing trailing cells are empty strings.
    /// </summary>
    /// <exception cref="CsvFormatException">The header is missing or a row is malformed.</exception>
    public static IEnumerable<Dictionary<string, string>> Read(TextReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);
        var headerLine = reader.ReadLine();
        if (string.IsNullOrEmpty(headerLine))
        {
            throw new CsvFormatException("CSV input has no header row");
        }

        var header = ParseLine(headerLine, reader, 1);
        var lineNumber = 1;
        string? line;
        while ((line = reader.ReadLine()) != null)
        {
            lineNumber++;
            if (line.Length == 0)
            {
                continue;
            }

            var cells = ParseLine(line, reader, lineNumber);
            var row = new Dictionary<string, string>(header.Count, StringComparer.Ordinal);
            for (var i = 0; i < header.Count; i++)
            {
                row[header[i]] = i < cells.Count ? cells[i] : "";
            }

            yield return row;
        }
    }

    /// <summary>
    /// DataWeave <c>write(payload, "application/csv", {header: false, lineSeparator: ""})</c>: one row as a single CSV
    /// line without a line terminator, cells in <paramref name="columns"/> order and quoted only when needed.
    /// </summary>
    public static string ToLine(IReadOnlyDictionary<string, string> row, IReadOnlyList<string> columns)
    {
        ArgumentNullException.ThrowIfNull(row);
        ArgumentNullException.ThrowIfNull(columns);
        var sb = new StringBuilder();
        for (var i = 0; i < columns.Count; i++)
        {
            if (i > 0)
            {
                sb.Append(Separator);
            }

            sb.Append(QuoteIfNeeded(row.TryGetValue(columns[i], out var v) ? v : ""));
        }

        return sb.ToString();
    }

    private static string QuoteIfNeeded(string value)
    {
        if (value.IndexOfAny([Separator, Quote, '\n', '\r']) < 0)
        {
            return value;
        }

        return Quote + value.Replace("\"", "\"\"", StringComparison.Ordinal) + Quote;
    }

    // Parses one record; a quoted cell may span physical lines, so the reader is consulted for continuations.
    private static List<string> ParseLine(string line, TextReader reader, int lineNumber)
    {
        var cells = new List<string>();
        var cell = new StringBuilder();
        var inQuotes = false;
        var i = 0;
        while (true)
        {
            if (i >= line.Length)
            {
                if (!inQuotes)
                {
                    break;
                }

                var next = reader.ReadLine()
                    ?? throw new CsvFormatException($"Unterminated quoted value starting on line {lineNumber}");
                cell.Append('\n');
                line = next;
                i = 0;
                continue;
            }

            var c = line[i];
            if (inQuotes)
            {
                if (c == Quote)
                {
                    if (i + 1 < line.Length && line[i + 1] == Quote)
                    {
                        cell.Append(Quote);
                        i += 2;
                        continue;
                    }

                    inQuotes = false;
                    i++;
                    continue;
                }

                cell.Append(c);
                i++;
                continue;
            }

            if (c == Quote && cell.Length == 0)
            {
                inQuotes = true;
                i++;
                continue;
            }

            if (c == Separator)
            {
                cells.Add(cell.ToString());
                cell.Clear();
                i++;
                continue;
            }

            cell.Append(c);
            i++;
        }

        cells.Add(cell.ToString());
        return cells;
    }
}

/// <summary>Thrown when the CSV input cannot be parsed; fails the whole file like a DataWeave read error would.</summary>
public sealed class CsvFormatException : Exception
{
    /// <summary>Creates the exception with a message.</summary>
    public CsvFormatException(string message) : base(message)
    {
    }
}
