using System.Text.Json.Serialization;

namespace Contacts.Api;

/// <summary>The <c>&lt;http:error-response&gt;</c> body: <c>{error: error.description}</c>.</summary>
public sealed record ErrorBody([property: JsonPropertyName("error")] string Error);
