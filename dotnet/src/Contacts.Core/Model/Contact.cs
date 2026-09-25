namespace Contacts.Core.Model;

/// <summary>
/// One row of the <c>contacts</c> table, typed the way the "CSV to SQL" DataWeave produced it
/// (<c>as Number</c> → <see cref="int"/>, <c>as Boolean</c> → <see cref="bool"/>, <c>as Date</c> → <see cref="DateOnly"/>;
/// the three timestamp columns become a <see cref="Timestamp"/>, date-only by default — MAPPING.md deviation 4).
/// </summary>
public sealed record Contact
{
    /// <summary>The CSV / table column names in the order of the upstream <c>INSERT</c> statement.</summary>
    public static readonly IReadOnlyList<string> Columns =
    [
        "account_id",
        "active_tracker_count",
        "assistant_name",
        "assistant_phone",
        "birthdate",
        "can_allow_portal_self_reg",
        "contact_id",
        "contact_name",
        "created_by_id",
        "created_date",
        "currency_iso_code",
        "department",
        "do_not_call",
        "email",
        "external_id",
        "fax",
        "first_name",
        "has_opted_out_of_email",
        "has_opted_out_of_fax",
        "has_privacy_hold",
        "home_phone",
        "is_deleted",
        "is_email_bounced",
        "is_locked",
        "is_person_account",
        "is_priority_record",
        "last_modified_by_id",
        "last_modified_date",
        "last_name",
        "lead_source",
        "mailing_city",
        "mailing_country",
        "mailing_postal_code",
        "mailing_state",
        "mailing_street",
        "may_edit",
        "middle_name",
        "mobile_phone",
        "other_city",
        "other_country",
        "other_phone",
        "other_postal_code",
        "other_state",
        "other_street",
        "owner_id",
        "phone",
        "photo_url",
        "record_type_id",
        "salutation",
        "suffix",
        "system_mod_stamp",
        "title",
    ];

#pragma warning disable CS1591 // The properties mirror the columns one-to-one; the names are the documentation.
    public string? AccountId { get; init; }
    public int? ActiveTrackerCount { get; init; }
    public string? AssistantName { get; init; }
    public string? AssistantPhone { get; init; }
    public DateOnly? Birthdate { get; init; }
    public bool? CanAllowPortalSelfReg { get; init; }
    public string? ContactId { get; init; }
    public string? ContactName { get; init; }
    public string? CreatedById { get; init; }
    public Timestamp? CreatedDate { get; init; }
    public string? CurrencyIsoCode { get; init; }
    public string? Department { get; init; }
    public bool? DoNotCall { get; init; }
    public string? Email { get; init; }
    public string? ExternalId { get; init; }
    public string? Fax { get; init; }
    public string? FirstName { get; init; }
    public bool? HasOptedOutOfEmail { get; init; }
    public bool? HasOptedOutOfFax { get; init; }
    public bool? HasPrivacyHold { get; init; }
    public string? HomePhone { get; init; }
    public bool? IsDeleted { get; init; }
    public bool? IsEmailBounced { get; init; }
    public bool? IsLocked { get; init; }
    public bool? IsPersonAccount { get; init; }
    public bool? IsPriorityRecord { get; init; }
    public string? LastModifiedById { get; init; }
    public Timestamp? LastModifiedDate { get; init; }
    public string? LastName { get; init; }
    public string? LeadSource { get; init; }
    public string? MailingCity { get; init; }
    public string? MailingCountry { get; init; }
    public string? MailingPostalCode { get; init; }
    public string? MailingState { get; init; }
    public string? MailingStreet { get; init; }
    public bool? MayEdit { get; init; }
    public string? MiddleName { get; init; }
    public string? MobilePhone { get; init; }
    public string? OtherCity { get; init; }
    public string? OtherCountry { get; init; }
    public string? OtherPhone { get; init; }
    public string? OtherPostalCode { get; init; }
    public string? OtherState { get; init; }
    public string? OtherStreet { get; init; }
    public string? OwnerId { get; init; }
    public string? Phone { get; init; }
    public string? PhotoUrl { get; init; }
    public string? RecordTypeId { get; init; }
    public string? Salutation { get; init; }
    public string? Suffix { get; init; }
    public Timestamp? SystemModStamp { get; init; }
    public string? Title { get; init; }
#pragma warning restore CS1591
}
