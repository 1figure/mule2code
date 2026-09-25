using System.Text.Json.Nodes;
using Contacts.Core.Batch;
using Contacts.Core.Csv;

namespace Contacts.Tests.Support;

/// <summary>Locates the shared <c>testdata/</c> directory of the repository; fixtures are read in place, never copied.</summary>
public static class TestData
{
    public static string Root { get; } = FindRoot();

    public static string Fixture(string relative) =>
        Path.Combine(Root, relative.Replace('/', Path.DirectorySeparatorChar));

    public static string ReadText(string relative) => File.ReadAllText(Fixture(relative));

    public static JsonNode ReadJson(string relative) =>
        JsonNode.Parse(ReadText(relative)) ?? throw new InvalidDataException($"{relative} is JSON null");

    public static List<Dictionary<string, string>> ReadRows(string relative)
    {
        using var reader = new StreamReader(Fixture(relative));
        return CsvRows.Read(reader).ToList();
    }

    /// <summary>Asserts the fields present in a <c>testdata/expected/batch-report-*.json</c> golden against the statistics.</summary>
    public static void AssertStatistics(string goldenRelative, BatchStatistics statistics)
    {
        var golden = ReadJson(goldenRelative).AsObject();
        var actual = JsonNode.Parse(System.Text.Json.JsonSerializer.Serialize(statistics))!.AsObject();
        Assert.All(golden, field =>
        {
            Assert.True(actual.ContainsKey(field.Key), $"statistics has no '{field.Key}'");
            Assert.True(field.Value!.GetValue<long>() == actual[field.Key]!.GetValue<long>(), $"{field.Key}: expected {field.Value} got {actual[field.Key]}");
        });
    }

    private static string FindRoot()
    {
        for (var dir = new DirectoryInfo(AppContext.BaseDirectory); dir != null; dir = dir.Parent)
        {
            var candidate = Path.Combine(dir.FullName, "testdata", "openapi.yaml");
            if (File.Exists(candidate))
            {
                return Path.Combine(dir.FullName, "testdata");
            }
        }

        throw new DirectoryNotFoundException("testdata/ not found above " + AppContext.BaseDirectory);
    }
}
