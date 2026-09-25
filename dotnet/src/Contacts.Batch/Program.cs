using System.Runtime.InteropServices;
using Contacts.Core.Data;
using Contacts.Core.Pipeline;
using Contacts.Core.Report;
using Contacts.Core.Settings;
using Microsoft.Extensions.Logging;

namespace Contacts.Batch;

/// <summary>
/// Console host of <c>batch-contacts-csv-to-db</c>. Without arguments it watches <c>&lt;DATA_DIR&gt;/inbox</c> like the
/// SFTP listener did; <c>--once</c> processes the inbox once; <c>--file &lt;path&gt;</c> processes one file.
/// In the one-shot modes the report goes to stdout, otherwise to <c>&lt;DATA_DIR&gt;/reports</c>.
/// </summary>
public static class Program
{
    private const string Usage = "usage: Contacts.Batch [--once | --file <path>] [--verbose]";

    /// <summary>Entry point; returns 0 on success, 1 on a failed file, 2 on bad arguments or configuration.</summary>
    public static async Task<int> Main(string[] args)
    {
        var once = false;
        string? file = null;
        var verbose = false;
        for (var i = 0; i < args.Length; i++)
        {
            switch (args[i])
            {
                case "--once":
                    once = true;
                    break;
                case "--file" when i + 1 < args.Length:
                    file = args[++i];
                    break;
                case "--verbose":
                    verbose = true;
                    break;
                default:
                    Console.Error.WriteLine(Usage);
                    return 2;
            }
        }

        using var loggerFactory = LoggerFactory.Create(logging =>
        {
            logging.SetMinimumLevel(verbose ? LogLevel.Debug : LogLevel.Information);
            logging.AddSimpleConsole(options =>
            {
                options.SingleLine = true;
                options.TimestampFormat = "HH:mm:ss.fff ";
            });
        });
        var logger = loggerFactory.CreateLogger("Contacts.Batch");

        // Ctrl-C and SIGTERM (docker stop, systemd) both stop cleanly between files instead of mid-file.
        using var shutdown = new CancellationTokenSource();
        Console.CancelKeyPress += (_, e) =>
        {
            e.Cancel = true;
            shutdown.Cancel();
        };
        using var sigterm = PosixSignalRegistration.Create(PosixSignal.SIGTERM, context =>
        {
            context.Cancel = true;
            shutdown.Cancel();
        });

        try
        {
            var settings = AppSettings.FromEnvironment();
            var repository = await SqlContactRepository.OpenAsync(settings.DbProfile, settings.ConnectionString, shutdown.Token);
            IReportSink sink = once || file != null ? new ConsoleReportSink() : new FileReportSink(settings.ReportsDir);
            var pipeline = new ContactsPipeline(settings, repository, sink, loggerFactory.CreateLogger<ContactsPipeline>());

            if (file != null)
            {
                await pipeline.ProcessFileAsync(file, shutdown.Token);
            }
            else if (once)
            {
                await pipeline.ProcessInboxAsync(continueOnError: false, shutdown.Token);
            }
            else
            {
                logger.LogInformation("Watching {InboxDir} every {PollSeconds}s ({DbProfile})", settings.InboxDir, settings.PollSeconds, settings.DbProfile);
                await pipeline.WatchAsync(shutdown.Token);
            }

            return 0;
        }
        catch (OperationCanceledException) when (shutdown.IsCancellationRequested)
        {
            logger.LogInformation("Stopped");
            return 0;
        }
        catch (SettingsException e)
        {
            logger.LogError("{Message}", e.Message);
            return 2;
        }
        catch (RepositoryException e)
        {
            logger.LogError(e, "{Message}", e.Message);
            return 2;
        }
        catch (PipelineException e)
        {
            // Already logged by the pipeline with the cause; the file is in failed/.
            logger.LogError("{Message}", e.Message);
            return 1;
        }
    }
}
