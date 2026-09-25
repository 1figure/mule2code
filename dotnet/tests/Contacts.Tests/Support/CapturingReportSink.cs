using Contacts.Core.Report;

namespace Contacts.Tests.Support;

/// <summary>An <see cref="IReportSink"/> that keeps every report it was given.</summary>
public sealed class CapturingReportSink : IReportSink
{
    public List<(string Subject, string Html)> Reports { get; } = [];

    public Task SendAsync(string subject, string html, CancellationToken cancellationToken)
    {
        Reports.Add((subject, html));
        return Task.CompletedTask;
    }
}
