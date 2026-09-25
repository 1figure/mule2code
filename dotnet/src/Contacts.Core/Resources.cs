namespace Contacts.Core;

/// <summary>
/// Locates the upstream files the port reuses at run time (the two schema scripts and the e-mail template), which
/// <c>Contacts.Core.csproj</c> copies next to the binaries.
/// </summary>
public static class Resources
{
    /// <summary>Path of <c>docker/sqlite/contacts.sqlite.sql</c>.</summary>
    public static string SqliteSchema => Locate("contacts.sqlite.sql");

    /// <summary>Path of <c>docker/init/01-contacts.sql</c>.</summary>
    public static string PostgresSchema => Locate("01-contacts.sql");

    /// <summary>Path of the upstream <c>parse-template/contacts-batch-report-email.template</c>.</summary>
    public static string ReportTemplate => Locate("contacts-batch-report-email.template");

    private static string Locate(string name)
    {
        var path = Path.Combine(AppContext.BaseDirectory, "Resources", name);
        return File.Exists(path)
            ? path
            : throw new FileNotFoundException($"Resource '{name}' was not copied to the output directory", path);
    }
}
