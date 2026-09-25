using Contacts.Tests.Support;

namespace Contacts.Tests.Batch;

/// <summary>The console host end to end: arguments, exit codes, stdout report. Uses the process environment, so it runs alone.</summary>
[Collection("process-environment")]
public sealed class ProgramTests : IDisposable
{
    private readonly TempDir dir = new();
    private readonly TextWriter originalOut = Console.Out;
    private readonly StringWriter output = new();

    public ProgramTests()
    {
        Environment.SetEnvironmentVariable("DATA_DIR", dir.Path);
        Console.SetOut(output);
    }

    public void Dispose()
    {
        Console.SetOut(originalOut);
        Environment.SetEnvironmentVariable("DATA_DIR", null);
        Environment.SetEnvironmentVariable("DB_PROFILE", null);
        output.Dispose();
        dir.Dispose();
    }

    [Fact]
    public async Task OnceProcessesTheInboxAndPrintsTheReport()
    {
        File.Copy(TestData.Fixture("contact-data-100-with-errors.csv"), Path.Combine(dir.Sub("inbox"), "contact-data-100-with-errors.csv"));

        var rc = await Contacts.Batch.Program.Main(["--once"]);

        Assert.Equal(0, rc);
        Assert.Contains("Subject: Mule Contacts Batch Job Report", output.ToString(), StringComparison.Ordinal);
        Assert.Contains("<tr><td>Successful records:</td><td>98</td></tr><tr><td>Failed records:</td><td>2</td></tr>", output.ToString(), StringComparison.Ordinal);
        Assert.Single(Directory.GetFiles(Path.Combine(dir.Path, "processed"), "*.csv"));
        Assert.Single(Directory.GetFiles(Path.Combine(dir.Path, "processed"), "*.errors.json"));
        Assert.True(File.Exists(Path.Combine(dir.Path, "contacts.db")));
    }

    [Fact]
    public async Task FileProcessesOneFileWithVerboseLogging()
    {
        var file = Path.Combine(dir.Sub("elsewhere"), "contact-data-100.csv");
        File.Copy(TestData.Fixture("contact-data-100.csv"), file);

        var rc = await Contacts.Batch.Program.Main(["--file", file, "--verbose"]);

        Assert.Equal(0, rc);
        Assert.False(File.Exists(file));
        Assert.Single(Directory.GetFiles(Path.Combine(dir.Path, "processed"), "contact-data-100.*.csv"));
    }

    [Fact]
    public async Task FailedFileReturns1AndLandsInFailed()
    {
        var file = Path.Combine(dir.Sub("inbox"), "broken.csv");
        await File.WriteAllTextAsync(file, "");

        var rc = await Contacts.Batch.Program.Main(["--file", file]);

        Assert.Equal(1, rc);
        Assert.True(File.Exists(Path.Combine(dir.Path, "failed", "broken.csv")));
    }

    [Theory]
    [InlineData("--bogus")]
    [InlineData("--file")]
    public async Task BadArgumentsReturn2(string arg)
    {
        var rc = await Contacts.Batch.Program.Main([arg]);

        Assert.Equal(2, rc);
    }

    [Fact]
    public async Task BadConfigurationReturns2()
    {
        Environment.SetEnvironmentVariable("DB_PROFILE", "oracle");

        var rc = await Contacts.Batch.Program.Main(["--once"]);

        Assert.Equal(2, rc);
    }

    [Fact]
    public async Task UnreachableDatabaseReturns2()
    {
        Environment.SetEnvironmentVariable("DB_PROFILE", "postgres");
        Environment.SetEnvironmentVariable("POSTGRES_CONNECTION_STRING", "Host=127.0.0.1;Port=1;Database=x;Username=x;Password=x;Timeout=1");
        try
        {
            var rc = await Contacts.Batch.Program.Main(["--once"]);

            Assert.Equal(2, rc);
        }
        finally
        {
            Environment.SetEnvironmentVariable("POSTGRES_CONNECTION_STRING", null);
        }
    }
}
