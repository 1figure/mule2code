using Contacts.Core.Batch;

namespace Contacts.Tests.Batch;

/// <summary>The <c>batch:job</c> semantics of the generic engine, with synthetic integer records.</summary>
public sealed class BatchJobTests
{
    private static BatchStep<int> Step(
        string name,
        AcceptPolicy accept = AcceptPolicy.NoFailures,
        RecordProcessor<int>? processor = null,
        BlockAggregator<int>? aggregator = null,
        int size = 0)
    {
        var step = new BatchStep<int>(name) { Accept = accept, Aggregator = aggregator, AggregatorSize = size };
        if (processor != null)
        {
            step.Processors.Add(processor);
        }

        return step;
    }

    private static RecordProcessor<int> FailWhen(Func<int, bool> predicate) => (record, _) =>
        predicate(record.Payload) ? throw new InvalidOperationException($"record {record.Payload} rejected") : Task.CompletedTask;

    private static BlockAggregator<int> Collect(List<List<int>> blocks) => (block, _) =>
    {
        blocks.Add(block.Select(r => r.Payload).ToList());
        return Task.CompletedTask;
    };

    [Fact]
    public async Task StepsRunInOrderAndEachFinishesForAllRecordsBeforeTheNextStarts()
    {
        var log = new List<string>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s1", processor: (r, _) => { log.Add($"s1:{r.Payload}"); return Task.CompletedTask; }));
        job.Steps.Add(Step("s2", processor: (r, _) => { log.Add($"s2:{r.Payload}"); return Task.CompletedTask; }));

        await job.RunAsync([1, 2, 3], CancellationToken.None);

        Assert.Equal(["s1:1", "s1:2", "s1:3", "s2:1", "s2:2", "s2:3"], log);
    }

    [Fact]
    public async Task AcceptPoliciesSelectRecordsByPreviousFailures()
    {
        var noFailures = new List<List<int>>();
        var onlyFailures = new List<List<int>>();
        var all = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("fail-evens", processor: FailWhen(n => n % 2 == 0)));
        job.Steps.Add(Step("no-failures", AcceptPolicy.NoFailures, aggregator: Collect(noFailures)));
        job.Steps.Add(Step("only-failures", AcceptPolicy.OnlyFailures, aggregator: Collect(onlyFailures)));
        job.Steps.Add(Step("all", AcceptPolicy.All, aggregator: Collect(all)));

        var stats = await job.RunAsync(Enumerable.Range(1, 6), CancellationToken.None);

        Assert.Equal([[1, 3, 5]], noFailures);
        Assert.Equal([[2, 4, 6]], onlyFailures);
        Assert.Equal([[1, 2, 3, 4, 5, 6]], all);
        Assert.Equal(3, stats.FailedRecords);
        Assert.Equal(3, stats.SuccessfulRecords);
    }

    [Fact]
    public async Task AggregatorReceivesBlocksOfTheConfiguredSize()
    {
        var blocks = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s", aggregator: Collect(blocks), size: 3));

        await job.RunAsync(Enumerable.Range(1, 10), CancellationToken.None);

        Assert.Equal([[1, 2, 3], [4, 5, 6], [7, 8, 9], [10]], blocks);
    }

    [Fact]
    public async Task AggregatorSizeZeroIsStreamingAllRecordsInOneBlock()
    {
        var blocks = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s", aggregator: Collect(blocks), size: 0));

        await job.RunAsync(Enumerable.Range(1, 10), CancellationToken.None);

        Assert.Equal([Enumerable.Range(1, 10).ToList()], blocks);
    }

    [Fact]
    public async Task AggregatorIsNotCalledWhenNoRecordReachesIt()
    {
        var blocks = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("fail-all", processor: FailWhen(_ => true)));
        job.Steps.Add(Step("s", aggregator: Collect(blocks)));

        await job.RunAsync([1, 2], CancellationToken.None);

        Assert.Empty(blocks);
    }

    [Fact]
    public async Task FailingAggregatorFailsExactlyItsBlock()
    {
        var failed = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s", aggregator: (block, _) =>
            block.Any(r => r.Payload == 5) ? throw new InvalidOperationException("block rejected") : Task.CompletedTask, size: 3));
        job.Steps.Add(Step("failed", AcceptPolicy.OnlyFailures, aggregator: Collect(failed)));

        var stats = await job.RunAsync(Enumerable.Range(1, 10), CancellationToken.None);

        Assert.Equal([[4, 5, 6]], failed);
        Assert.Equal(3, stats.FailedRecords);
        Assert.Equal(7, stats.SuccessfulRecords);
    }

    [Fact]
    public async Task FailedRecordKeepsItsFirstException()
    {
        BatchRecord<int>? seen = null;
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s1", processor: FailWhen(_ => true)));
        job.Steps.Add(Step("s2", AcceptPolicy.OnlyFailures, processor: (r, _) => throw new ArgumentException("second")));
        job.Steps.Add(Step("s3", AcceptPolicy.OnlyFailures, processor: (r, _) => { seen = r; return Task.CompletedTask; }));

        await job.RunAsync([1], CancellationToken.None);

        var error = Assert.IsType<InvalidOperationException>(seen!.Error);
        Assert.Equal("record 1 rejected", error.Message);
    }

    [Fact]
    public async Task RecordFailingInAStepsProcessorNeverReachesThatStepsAggregator()
    {
        var blocks = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s", processor: FailWhen(n => n == 2), aggregator: Collect(blocks)));

        await job.RunAsync([1, 2, 3], CancellationToken.None);

        Assert.Equal([[1, 3]], blocks);
    }

    [Fact]
    public async Task MaxFailedRecordsStopsTheJobOnceExceeded()
    {
        var processed = new List<int>();
        var later = new List<List<int>>();
        var job = new BatchJob<int>("job") { MaxFailedRecords = 1 };
        job.Steps.Add(Step("s", processor: (r, _) => { processed.Add(r.Payload); throw new InvalidOperationException("x"); }, aggregator: (_, _) => Task.CompletedTask));
        job.Steps.Add(Step("later", AcceptPolicy.All, aggregator: Collect(later)));

        var stats = await job.RunAsync(Enumerable.Range(1, 5), CancellationToken.None);

        Assert.Equal([1, 2], processed);
        Assert.Empty(later);
        Assert.Equal(2, stats.FailedRecords);
    }

    [Fact]
    public async Task MaxFailedRecordsStopsTheJobAfterAFailingAggregatorBlock()
    {
        var blocks = 0;
        var job = new BatchJob<int>("job") { MaxFailedRecords = 0 };
        job.Steps.Add(Step("s", aggregator: (_, _) => { blocks++; throw new InvalidOperationException("x"); }, size: 2));

        var stats = await job.RunAsync(Enumerable.Range(1, 6), CancellationToken.None);

        Assert.Equal(1, blocks);
        Assert.Equal(2, stats.FailedRecords);
    }

    [Fact]
    public async Task MinusOneMeansUnlimitedFailures()
    {
        var failed = new List<List<int>>();
        var job = new BatchJob<int>("job") { MaxFailedRecords = -1 };
        job.Steps.Add(Step("fail-all", processor: FailWhen(_ => true)));
        job.Steps.Add(Step("failed", AcceptPolicy.OnlyFailures, aggregator: Collect(failed)));

        var stats = await job.RunAsync(Enumerable.Range(1, 5), CancellationToken.None);

        Assert.Equal([[1, 2, 3, 4, 5]], failed);
        Assert.Equal(5, stats.FailedRecords);
        Assert.Equal(0, stats.SuccessfulRecords);
    }

    [Fact]
    public async Task StatisticsCountEveryRecordAndOnCompleteReceivesThem()
    {
        BatchStatistics? received = null;
        var job = new BatchJob<int>("job")
        {
            BlockSize = 4,
            OnComplete = (s, _) => { received = s; return Task.CompletedTask; },
        };
        job.Steps.Add(Step("s", processor: FailWhen(n => n <= 3)));

        var stats = await job.RunAsync(Enumerable.Range(1, 10), CancellationToken.None);

        Assert.Same(stats, received);
        Assert.Equal(10, stats.TotalRecords);
        Assert.Equal(10, stats.LoadedRecords);
        Assert.Equal(10, stats.ProcessedRecords);
        Assert.Equal(7, stats.SuccessfulRecords);
        Assert.Equal(3, stats.FailedRecords);
        Assert.False(stats.FailedOnInputPhase);
        Assert.True(Guid.TryParse(stats.BatchJobInstanceId, out _));
        Assert.True(stats.ElapsedTimeInMillis >= 0);
    }

    [Fact]
    public async Task EmptyInputCompletesWithZeroStatistics()
    {
        var completed = false;
        var blocks = new List<List<int>>();
        var job = new BatchJob<int>("job") { OnComplete = (_, _) => { completed = true; return Task.CompletedTask; } };
        job.Steps.Add(Step("s", aggregator: Collect(blocks)));

        var stats = await job.RunAsync([], CancellationToken.None);

        Assert.True(completed);
        Assert.Empty(blocks);
        Assert.Equal(0, stats.TotalRecords);
    }

    [Fact]
    public async Task OnCompleteFailureIsRecordedOnTheStatisticsNotThrown()
    {
        var job = new BatchJob<int>("job") { OnComplete = (_, _) => throw new IOException("smtp down") };
        job.Steps.Add(Step("s", processor: FailWhen(n => n == 1)));

        var stats = await job.RunAsync([1, 2, 3], CancellationToken.None);

        Assert.True(stats.FailedOnCompletePhase);
        Assert.Equal(2, stats.SuccessfulRecords);
        Assert.Equal(1, stats.FailedRecords);
    }

    [Fact]
    public async Task CancellationInsideOnCompleteStillPropagates()
    {
        using var cts = new CancellationTokenSource();
        var job = new BatchJob<int>("job")
        {
            OnComplete = async (_, ct) =>
            {
                await cts.CancelAsync();
                ct.ThrowIfCancellationRequested();
            },
        };

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() => job.RunAsync([1], cts.Token));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-5)]
    public async Task NonPositiveBlockSizeIsRejectedUpFront(int blockSize)
    {
        var job = new BatchJob<int>("job") { BlockSize = blockSize };

        await Assert.ThrowsAsync<ArgumentOutOfRangeException>(() => job.RunAsync([1], CancellationToken.None));
    }

    [Fact]
    public async Task InputPhaseErrorIsWrappedAndOnCompleteIsNotCalled()
    {
        var completed = false;
        var job = new BatchJob<int>("job") { OnComplete = (_, _) => { completed = true; return Task.CompletedTask; } };
        job.Steps.Add(Step("s", processor: (_, _) => Task.CompletedTask));

        var e = await Assert.ThrowsAsync<BatchInputException>(() => job.RunAsync(Throwing(), CancellationToken.None));

        Assert.IsType<InvalidDataException>(e.InnerException);
        Assert.Contains("job", e.Message, StringComparison.Ordinal);
        Assert.False(completed);

        static IEnumerable<int> Throwing()
        {
            yield return 1;
            throw new InvalidDataException("bad input");
        }
    }

    [Fact]
    public async Task CancellationInsideAProcessorStopsTheJob()
    {
        using var cts = new CancellationTokenSource();
        var completed = false;
        var job = new BatchJob<int>("job") { OnComplete = (_, _) => { completed = true; return Task.CompletedTask; } };
        job.Steps.Add(Step("s", processor: (r, ct) =>
        {
            if (r.Payload == 2)
            {
                cts.Cancel();
            }

            ct.ThrowIfCancellationRequested();
            return Task.CompletedTask;
        }));

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() => job.RunAsync([1, 2, 3], cts.Token));

        Assert.False(completed);
    }

    [Fact]
    public async Task AlreadyCancelledTokenStopsBeforeProcessing()
    {
        using var cts = new CancellationTokenSource();
        await cts.CancelAsync();
        var processed = 0;
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s", processor: (_, _) => { processed++; return Task.CompletedTask; }));

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() => job.RunAsync([1, 2, 3], cts.Token));

        Assert.Equal(0, processed);
    }

    [Fact]
    public async Task ProcessorMayReplaceThePayloadSeenByTheAggregator()
    {
        var blocks = new List<List<int>>();
        var job = new BatchJob<int>("job");
        job.Steps.Add(Step("s", processor: (r, _) => { r.Payload *= 10; return Task.CompletedTask; }, aggregator: Collect(blocks)));

        await job.RunAsync([1, 2], CancellationToken.None);

        Assert.Equal([[10, 20]], blocks);
    }
}
