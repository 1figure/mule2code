using System.Xml.Linq;
using Contacts.Tests.Support;

namespace Contacts.Tests.Mapping;

/// <summary>
/// MAPPING.md as a check: every flow, batch step/job, and every connector, transform and validation element of
/// the two Mule implementation files must be named in a comment somewhere under <c>src/</c>. Loggers,
/// set-variables and error-handler branches are excluded; they have no code counterpart of their own.
/// </summary>
public sealed class MuleTraceabilityTests
{
    private static readonly XNamespace Core = "http://www.mulesoft.org/schema/mule/core";
    private static readonly XNamespace Doc = "http://www.mulesoft.org/schema/mule/documentation";

    private static readonly XNamespace[] ConnectorNamespaces =
    [
        "http://www.mulesoft.org/schema/mule/batch",
        "http://www.mulesoft.org/schema/mule/ee/core",
        "http://www.mulesoft.org/schema/mule/validation",
        "http://www.mulesoft.org/schema/mule/db",
        "http://www.mulesoft.org/schema/mule/http",
        "http://www.mulesoft.org/schema/mule/sftp",
        "http://www.mulesoft.org/schema/mule/email",
    ];

    // set-payload: contacts-api-impl.xml was changed after generation from <ee:transform> (EE-only) to
    // <set-payload> with the same DataWeave and the same doc:names (mule/contacts-api/README.md).
    private static readonly string[] CoreElementsWithDocName = ["parse-template", "set-payload", "foreach", "try", "on-error-continue"];

    private static readonly string[] ImplementationFiles =
    [
        "mule/batch-contacts-csv-to-db/src/main/mule/batch-contacts-csv-to-db-impl.xml",
        "mule/contacts-api/src/main/mule/contacts-api-impl.xml",
    ];

    private static readonly Lazy<string> SourceText = new(() =>
        string.Join('\n', Directory.EnumerateFiles(Path.Combine(TestData.Root, "..", "dotnet", "src"), "*.cs", SearchOption.AllDirectories)
            .Where(f => !f.Contains($"{Path.DirectorySeparatorChar}obj{Path.DirectorySeparatorChar}", StringComparison.Ordinal))
            .Select(File.ReadAllText)));

    public static TheoryData<string, string> MuleElements()
    {
        var data = new TheoryData<string, string>();
        foreach (var relative in ImplementationFiles)
        {
            var file = Path.GetFileName(relative);
            var root = XDocument.Load(Path.Combine(TestData.Root, "..", relative)).Root!;
            foreach (var element in root.Descendants())
            {
                var name = element.Name;
                if (name == Core + "flow" || name == Core + "sub-flow")
                {
                    data.Add(file, $"flow name=\"{element.Attribute("name")!.Value}\"");
                }
                else if (name.Namespace == "http://www.mulesoft.org/schema/mule/batch" && (name.LocalName == "step" || name.LocalName == "job"))
                {
                    var attribute = name.LocalName == "job" ? "jobName" : "name";
                    data.Add(file, $"batch:{name.LocalName} {attribute}=\"{element.Attribute(attribute)!.Value}\"");
                }
                else if (element.Attribute(Doc + "name") is { } docName
                    && (ConnectorNamespaces.Contains(name.Namespace) || (name.Namespace == Core && CoreElementsWithDocName.Contains(name.LocalName))))
                {
                    data.Add(file, $"{Prefix(name)}{name.LocalName} doc:name=\"{docName.Value}\"");
                }
            }
        }

        return data;
    }

    [Theory]
    [MemberData(nameof(MuleElements))]
    public void EveryMuleElementIsNamedInTheSource(string file, string element)
    {
        // The comment must contain the element's identifying attribute value, e.g. the doc:name text.
        var value = element[(element.IndexOf('"', StringComparison.Ordinal) + 1)..^1];

        Assert.True(SourceText.Value.Contains(value, StringComparison.Ordinal), $"{file}: <{element}> is not named anywhere under dotnet/src");
    }

    [Fact]
    public void TheCheckCoversBothImplementationFiles()
    {
        var files = MuleElements().Select(row => (string)row[0]).Distinct().ToList();

        Assert.Equal(2, files.Count);
        Assert.True(MuleElements().Count() >= 30, "expected the two XML files to yield at least 30 traceable elements");
    }

    private static string Prefix(XName name) => name.Namespace.NamespaceName switch
    {
        "http://www.mulesoft.org/schema/mule/core" => "",
        "http://www.mulesoft.org/schema/mule/ee/core" => "ee:",
        var ns => ns[(ns.LastIndexOf('/') + 1)..] + ":",
    };
}
