using Contacts.Core.Model;

namespace Contacts.Core.Mapping;

/// <summary>The "CSV to SQL" DataWeave transform inside <c>main-records-aggregator</c>, one record at a time.</summary>
public static class ContactMapper
{
    /// <summary>
    /// <c>&lt;ee:transform doc:name="CSV to SQL"&gt;</c>: maps one CSV row (header → cell) to a typed <see cref="Contact"/>.
    /// Text cells pass through unchanged, including empty strings (MAPPING.md deviation 6).
    /// </summary>
    /// <param name="row">The CSV row.</param>
    /// <param name="precision">How the three timestamp columns are coerced (<c>TIMESTAMP_PRECISION</c>); Mule's <c>as Date</c> by default.</param>
    /// <exception cref="CoercionException">A typed field does not parse.</exception>
    public static Contact FromRow(IReadOnlyDictionary<string, string> row, TimestampPrecision precision = TimestampPrecision.Date)
    {
        ArgumentNullException.ThrowIfNull(row);
        string? S(string key) => row.TryGetValue(key, out var v) ? v : null;

        return new Contact
        {
            AccountId = S("account_id"),
            ActiveTrackerCount = Coerce.ToInt(S("active_tracker_count"), "active_tracker_count"),
            AssistantName = S("assistant_name"),
            AssistantPhone = S("assistant_phone"),
            Birthdate = Coerce.ToDate(S("birthdate"), "birthdate"),
            CanAllowPortalSelfReg = Coerce.ToBool(S("can_allow_portal_self_reg"), "can_allow_portal_self_reg"),
            ContactId = S("contact_id"),
            ContactName = S("contact_name"),
            CreatedById = S("created_by_id"),
            CreatedDate = Coerce.ToTimestamp(S("created_date"), "created_date", precision),
            CurrencyIsoCode = S("currency_iso_code"),
            Department = S("department"),
            DoNotCall = Coerce.ToBool(S("do_not_call"), "do_not_call"),
            Email = S("email"),
            ExternalId = S("external_id"),
            Fax = S("fax"),
            FirstName = S("first_name"),
            HasOptedOutOfEmail = Coerce.ToBool(S("has_opted_out_of_email"), "has_opted_out_of_email"),
            HasOptedOutOfFax = Coerce.ToBool(S("has_opted_out_of_fax"), "has_opted_out_of_fax"),
            HasPrivacyHold = Coerce.ToBool(S("has_privacy_hold"), "has_privacy_hold"),
            HomePhone = S("home_phone"),
            IsDeleted = Coerce.ToBool(S("is_deleted"), "is_deleted"),
            IsEmailBounced = Coerce.ToBool(S("is_email_bounced"), "is_email_bounced"),
            IsLocked = Coerce.ToBool(S("is_locked"), "is_locked"),
            IsPersonAccount = Coerce.ToBool(S("is_person_account"), "is_person_account"),
            IsPriorityRecord = Coerce.ToBool(S("is_priority_record"), "is_priority_record"),
            LastModifiedById = S("last_modified_by_id"),
            LastModifiedDate = Coerce.ToTimestamp(S("last_modified_date"), "last_modified_date", precision),
            LastName = S("last_name"),
            LeadSource = S("lead_source"),
            MailingCity = S("mailing_city"),
            MailingCountry = S("mailing_country"),
            MailingPostalCode = S("mailing_postal_code"),
            MailingState = S("mailing_state"),
            MailingStreet = S("mailing_street"),
            MayEdit = Coerce.ToBool(S("may_edit"), "may_edit"),
            MiddleName = S("middle_name"),
            MobilePhone = S("mobile_phone"),
            OtherCity = S("other_city"),
            OtherCountry = S("other_country"),
            OtherPhone = S("other_phone"),
            OtherPostalCode = S("other_postal_code"),
            OtherState = S("other_state"),
            OtherStreet = S("other_street"),
            OwnerId = S("owner_id"),
            Phone = S("phone"),
            PhotoUrl = S("photo_url"),
            RecordTypeId = S("record_type_id"),
            Salutation = S("salutation"),
            Suffix = S("suffix"),
            SystemModStamp = Coerce.ToTimestamp(S("system_mod_stamp"), "system_mod_stamp", precision),
            Title = S("title"),
        };
    }
}
