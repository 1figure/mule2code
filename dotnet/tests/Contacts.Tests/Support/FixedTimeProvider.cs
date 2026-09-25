namespace Contacts.Tests.Support;

/// <summary>A <see cref="TimeProvider"/> stuck at one instant, reported in UTC so a zero-offset value round-trips.</summary>
public sealed class FixedTimeProvider(DateTimeOffset now) : TimeProvider
{
    public override DateTimeOffset GetUtcNow() => now.ToUniversalTime();

    public override TimeZoneInfo LocalTimeZone => TimeZoneInfo.Utc;
}
