using System.Text.Json;
using System.Text.Json.Nodes;
using YamlDotNet.Serialization;

namespace Contacts.Tests.Support;

/// <summary>
/// Reads <c>testdata/openapi.yaml</c> and asserts that a JSON body carries every <c>required</c> property of a schema,
/// recursively through <c>$ref</c>, inline objects and arrays, with scalar types checked.
/// </summary>
public sealed class OpenApiContract
{
    private readonly Dictionary<object, object> schemas;
    private readonly Dictionary<object, object> errorSchema;

    public OpenApiContract()
    {
        var root = new DeserializerBuilder().Build().Deserialize<Dictionary<object, object>>(TestData.ReadText("openapi.yaml"));
        var components = Map(root["components"]);
        schemas = Map(components["schemas"]);
        errorSchema = Map(Map(Map(Map(Map(components["responses"])["Error"])["content"])["application/json"])["schema"]);
    }

    public void AssertConforms(string json, string schemaName) =>
        AssertSchema(JsonNode.Parse(json), Map(schemas[schemaName]), schemaName);

    public void AssertErrorBody(string json) => AssertSchema(JsonNode.Parse(json), errorSchema, "Error");

    private static Dictionary<object, object> Map(object value) => (Dictionary<object, object>)value;

    private void AssertSchema(JsonNode? node, Dictionary<object, object> schema, string path)
    {
        if (schema.TryGetValue("$ref", out var reference))
        {
            var name = ((string)reference).Split('/')[^1];
            schema = Map(schemas[name]);
        }

        if (node is null)
        {
            Assert.True(schema.TryGetValue("nullable", out var nullable) && (string)nullable == "true", $"{path} is null but not nullable");
            return;
        }

        var type = schema.TryGetValue("type", out var t) ? (string)t : "object";
        switch (type)
        {
            case "object":
                var obj = Assert.IsType<JsonObject>(node);
                if (schema.TryGetValue("required", out var required))
                {
                    foreach (var name in (List<object>)required)
                    {
                        Assert.True(obj.ContainsKey((string)name), $"{path}.{name} is missing");
                    }
                }

                if (schema.TryGetValue("properties", out var properties))
                {
                    foreach (var (name, propertySchema) in Map(properties))
                    {
                        if (obj.TryGetPropertyValue((string)name, out var child))
                        {
                            AssertSchema(child, Map(propertySchema), $"{path}.{name}");
                        }
                    }
                }

                break;
            case "array":
                var array = Assert.IsType<JsonArray>(node);
                var items = Map(schema["items"]);
                for (var i = 0; i < array.Count; i++)
                {
                    AssertSchema(array[i], items, $"{path}[{i}]");
                }

                break;
            case "string":
                Assert.True(node.GetValueKind() == JsonValueKind.String, $"{path} should be a string");
                break;
            case "integer":
            case "number":
                Assert.True(node.GetValueKind() == JsonValueKind.Number, $"{path} should be a number");
                break;
            case "boolean":
                Assert.True(node.GetValueKind() is JsonValueKind.True or JsonValueKind.False, $"{path} should be a boolean");
                break;
            default:
                throw new NotSupportedException($"{path}: schema type '{type}'");
        }
    }
}
