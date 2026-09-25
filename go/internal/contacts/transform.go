package contacts

// FromRow is <ee:transform doc:name="CSV to SQL"> inside main-records-aggregator, for one record:
// it maps a CSV row (header → cell) to a typed Contact. Text cells pass through unchanged,
// including empty strings (MAPPING.md deviation 6); the three timestamp columns follow precision
// (an empty Precision means PrecisionDate, Mule's `as Date`); a typed field that does not parse returns a
// *CoercionError, the first one in column order.
func FromRow(row Row, precision Precision) (Contact, error) {
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	text := func(key string) *string {
		if v, ok := row[key]; ok {
			return &v
		}
		return nil
	}
	number := func(key string) *int64 {
		v, err := parseInt(text(key), key)
		keep(err)
		return v
	}
	boolean := func(key string) *bool {
		v, err := parseBool(text(key), key)
		keep(err)
		return v
	}
	date := func(key string) *Date {
		v, err := parseDate(text(key), key)
		keep(err)
		return v
	}
	timestamp := func(key string) *Timestamp {
		v, err := parseTimestamp(text(key), key, precision)
		keep(err)
		return v
	}

	c := Contact{
		AccountID:             text("account_id"),
		ActiveTrackerCount:    number("active_tracker_count"),
		AssistantName:         text("assistant_name"),
		AssistantPhone:        text("assistant_phone"),
		Birthdate:             date("birthdate"),
		CanAllowPortalSelfReg: boolean("can_allow_portal_self_reg"),
		ContactID:             text("contact_id"),
		ContactName:           text("contact_name"),
		CreatedByID:           text("created_by_id"),
		CreatedDate:           timestamp("created_date"),
		CurrencyISOCode:       text("currency_iso_code"),
		Department:            text("department"),
		DoNotCall:             boolean("do_not_call"),
		Email:                 text("email"),
		ExternalID:            text("external_id"),
		Fax:                   text("fax"),
		FirstName:             text("first_name"),
		HasOptedOutOfEmail:    boolean("has_opted_out_of_email"),
		HasOptedOutOfFax:      boolean("has_opted_out_of_fax"),
		HasPrivacyHold:        boolean("has_privacy_hold"),
		HomePhone:             text("home_phone"),
		IsDeleted:             boolean("is_deleted"),
		IsEmailBounced:        boolean("is_email_bounced"),
		IsLocked:              boolean("is_locked"),
		IsPersonAccount:       boolean("is_person_account"),
		IsPriorityRecord:      boolean("is_priority_record"),
		LastModifiedByID:      text("last_modified_by_id"),
		LastModifiedDate:      timestamp("last_modified_date"),
		LastName:              text("last_name"),
		LeadSource:            text("lead_source"),
		MailingCity:           text("mailing_city"),
		MailingCountry:        text("mailing_country"),
		MailingPostalCode:     text("mailing_postal_code"),
		MailingState:          text("mailing_state"),
		MailingStreet:         text("mailing_street"),
		MayEdit:               boolean("may_edit"),
		MiddleName:            text("middle_name"),
		MobilePhone:           text("mobile_phone"),
		OtherCity:             text("other_city"),
		OtherCountry:          text("other_country"),
		OtherPhone:            text("other_phone"),
		OtherPostalCode:       text("other_postal_code"),
		OtherState:            text("other_state"),
		OtherStreet:           text("other_street"),
		OwnerID:               text("owner_id"),
		Phone:                 text("phone"),
		PhotoURL:              text("photo_url"),
		RecordTypeID:          text("record_type_id"),
		Salutation:            text("salutation"),
		Suffix:                text("suffix"),
		SystemModStamp:        timestamp("system_mod_stamp"),
		Title:                 text("title"),
	}
	if firstErr != nil {
		return Contact{}, firstErr
	}
	return c, nil
}
