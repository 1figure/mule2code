using System.Text.Json;
using System.Text.Json.Nodes;

namespace Contacts.Tests.Support;

/// <summary>JSON normalisation for byte-for-byte comparisons: object keys sorted, indented, values untouched.</summary>
public static class Json
{
    private static readonly JsonSerializerOptions Indented = new() { WriteIndented = true };

    public static string Canonical(string json) => Canonical(JsonNode.Parse(json));

    public static string Canonical(JsonNode? node) => Sort(node)?.ToJsonString(Indented) ?? "null";

    public static string Canonical<T>(T value, JsonSerializerOptions? options = null) =>
        Canonical(JsonSerializer.Serialize(value, options));

    private static JsonNode? Sort(JsonNode? node) => node switch
    {
        JsonObject o => new JsonObject(o
            .OrderBy(p => p.Key, StringComparer.Ordinal)
            .Select(p => KeyValuePair.Create(p.Key, Sort(p.Value)))),
        JsonArray a => new JsonArray(a.Select(Sort).ToArray()),
        null => null,
        _ => node.DeepClone(),
    };
}
