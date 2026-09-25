namespace Contacts.Core.Data;

/// <summary>The columns selected by <c>&lt;db:select doc:name="Select contacts by city"&gt;</c>.</summary>
public sealed record ContactRow
{
#pragma warning disable CS1591 // Column names, one-to-one.
    public string? ContactId { get; init; }
    public string? FirstName { get; init; }
    public string? LastName { get; init; }
    public string? Email { get; init; }
    public string? Phone { get; init; }
    public string? Title { get; init; }
    public string? Department { get; init; }
    public string? MailingStreet { get; init; }
    public string? MailingCity { get; init; }
    public string? MailingState { get; init; }
    public string? MailingPostalCode { get; init; }
    public string? MailingCountry { get; init; }
#pragma warning restore CS1591
}
