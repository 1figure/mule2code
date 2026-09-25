using System.Globalization;

namespace Contacts.Core.Report;

/// <summary>
/// Where the rendered report goes; replaces <c>&lt;email:send doc:name="Contacts Batch Report Email"&gt;</c>
/// (MAPPING.md deviation 2). Real SMTP would be one MailKit call behind this interface.
/// </summary>
public interface IReportSink
{
    /// <summary>Delivers one report.</summary>
    Task SendAsync(string subject, string html, CancellationToken cancellationToken);
}

/// <summary>Writes each report to <c>&lt;directory&gt;/&lt;timestamp&gt;.html</c>.</summary>
public sealed class FileReportSink : IReportSink
{
    private readonly string directory;
    private readonly TimeProvider clock;

    /// <summary>Creates a sink writing into <paramref name="directory"/> (created on first use).</summary>
    public FileReportSink(string directory, TimeProvider? timeProvider = null)
    {
        ArgumentException.ThrowIfNullOrEmpty(directory);
        this.directory = directory;
        clock = timeProvider ?? TimeProvider.System;
    }

    /// <inheritdoc />
    public async Task SendAsync(string subject, string html, CancellationToken cancellationToken)
    {
        Directory.CreateDirectory(directory);
        var name = clock.GetLocalNow().ToString("yyyyMMdd'T'HHmmssfff", CultureInfo.InvariantCulture) + ".html";
        var path = Path.Combine(directory, name);
        var content = $"<!-- {subject} -->\n{html}";
        await File.WriteAllTextAsync(path, content, cancellationToken).ConfigureAwait(false);
    }
}

/// <summary>Prints the report to a <see cref="TextWriter"/> (stdout in one-shot mode).</summary>
public sealed class ConsoleReportSink : IReportSink
{
    private readonly TextWriter writer;

    /// <summary>Creates a sink printing to <paramref name="writer"/>, or <see cref="Console.Out"/>.</summary>
    public ConsoleReportSink(TextWriter? writer = null)
    {
        this.writer = writer ?? Console.Out;
    }

    /// <inheritdoc />
    public async Task SendAsync(string subject, string html, CancellationToken cancellationToken)
    {
        await writer.WriteLineAsync($"Subject: {subject}".AsMemory(), cancellationToken).ConfigureAwait(false);
        await writer.WriteLineAsync(html.AsMemory(), cancellationToken).ConfigureAwait(false);
        await writer.FlushAsync(cancellationToken).ConfigureAwait(false);
    }
}
