package i18n

// Message keys of i18n/locales/*.yaml, for text the backend translates itself:
// errors returned to AI clients, emails and notifications. Web console messages
// are not listed here; the web console translates them (see common.Message).

// Errors returned to AI clients
const (
	MsgDatabaseError          = "common.database_error"
	MsgAuthUserBanned         = "auth.user_banned"
	MsgTokenGetInfoFailed     = "token.get_info_failed"
	MsgTokenInvalid           = "token.invalid"
	MsgTokenNotProvided       = "token.not_provided"
	MsgTokenStatusUnavailable = "token.status_unavailable"

	MsgAuthClientIPUnresolved     = "auth.client_ip_unresolved"
	MsgAuthTokenIPNotAllowed      = "auth.token_ip_not_allowed"
	MsgAuthTokenGroupForbidden    = "auth.token_group_forbidden"
	MsgAuthTokenGroupDeprecated   = "auth.token_group_deprecated"
	MsgAuthChannelSelectForbidden = "auth.channel_select_forbidden"

	MsgRateLimitReached      = "rate_limit.reached"
	MsgRateLimitTotalReached = "rate_limit.total_reached"

	MsgDistributorInvalidRequest               = "distributor.invalid_request"
	MsgDistributorInvalidChannelId             = "distributor.invalid_channel_id"
	MsgDistributorChannelDisabled              = "distributor.channel_disabled"
	MsgDistributorTokenNoModelAccess           = "distributor.token_no_model_access"
	MsgDistributorTokenModelForbidden          = "distributor.token_model_forbidden"
	MsgDistributorModelNameRequired            = "distributor.model_name_required"
	MsgDistributorInvalidPlayground            = "distributor.invalid_playground_request"
	MsgDistributorGroupAccessDenied            = "distributor.group_access_denied"
	MsgDistributorGetChannelFailed             = "distributor.get_channel_failed"
	MsgDistributorNoAvailableChannel           = "distributor.no_available_channel"
	MsgDistributorNoAvailableChannelTaskPlugin = "distributor.no_available_channel_task_plugin"
	MsgDistributorInvalidMidjourney            = "distributor.invalid_midjourney_request"
	MsgDistributorInvalidParseModel            = "distributor.invalid_request_parse_model"

	MsgQuotaUserInsufficient             = "quota.user_insufficient"
	MsgQuotaPreConsumeInsufficient       = "quota.pre_consume_insufficient"
	MsgQuotaSubscriptionInsufficient     = "quota.subscription_insufficient"
	MsgRelayModelPriceNotConfigured      = "relay.model_price_not_configured"
	MsgRelayModelPriceNotConfiguredAdmin = "relay.model_price_not_configured_admin"
	MsgRelayTaskChannelDisabled          = "relay.task_channel_disabled"
	MsgRelayGroupSaturated               = "relay.group_saturated"
	MsgRelayGroupUpstreamSaturated       = "relay.group_upstream_saturated"
	MsgRelayRetryGetChannelFailed        = "relay.retry_get_channel_failed"
	MsgRelayRetryNoAvailableChannel      = "relay.retry_no_available_channel"
	MsgRelayAccessTokenUnsupported       = "relay.access_token_unsupported"
)

// Emails and the name of the token created at registration, in the
// recipient's language
const (
	MsgUserDefaultTokenName = "user.default_token_name"

	MsgEmailVerificationSubject  = "email.verification.subject"
	MsgEmailVerificationGreeting = "email.verification.greeting"
	MsgEmailVerificationCode     = "email.verification.code"
	MsgEmailVerificationValidity = "email.verification.validity"

	MsgEmailPasswordResetSubject      = "email.password_reset.subject"
	MsgEmailPasswordResetGreeting     = "email.password_reset.greeting"
	MsgEmailPasswordResetLink         = "email.password_reset.link"
	MsgEmailPasswordResetLinkFallback = "email.password_reset.link_fallback"
	MsgEmailPasswordResetValidity     = "email.password_reset.validity"
)

// Low-quota notices, in the recipient's saved language
const (
	MsgNotifyQuotaLowTitle             = "notify.quota_low_title"
	MsgNotifySubscriptionQuotaLowTitle = "notify.subscription_quota_low_title"
	MsgNotifyQuotaLowBark              = "notify.quota_low_bark"
	MsgNotifyQuotaLowGotify            = "notify.quota_low_gotify"
	MsgNotifyQuotaLowEmail             = "notify.quota_low_email"
)

// Channel notices sent to the root user and model update watchers
const (
	MsgChannelNotifyDisabledSubject = "channel.notify_disabled_subject"
	MsgChannelNotifyDisabledContent = "channel.notify_disabled_content"
	MsgChannelNotifyEnabledSubject  = "channel.notify_enabled_subject"
	MsgChannelNotifyEnabledContent  = "channel.notify_enabled_content"
	MsgChannelTestCompletedSubject  = "channel.test_completed_subject"
	MsgChannelTestCompletedContent  = "channel.test_completed_content"
	MsgChannelResponseTimeExceeded  = "channel.response_time_exceeded"

	MsgChannelUpstreamUpdateNotifySubject   = "channel.upstream_update_notify_subject"
	MsgChannelUpstreamUpdateSummary         = "channel.upstream_update_summary"
	MsgChannelUpstreamUpdateChangedChannels = "channel.upstream_update_changed_channels"
	MsgChannelUpstreamUpdateMoreChannels    = "channel.upstream_update_more_channels"
	MsgChannelUpstreamUpdateAddedModels     = "channel.upstream_update_added_models"
	MsgChannelUpstreamUpdateRemovedModels   = "channel.upstream_update_removed_models"
	MsgChannelUpstreamUpdateFailedChannels  = "channel.upstream_update_failed_channels"
	MsgChannelUpstreamUpdateMoreOmitted     = "channel.upstream_update_more_omitted"
)

// Balance ledger PDF export labels
const (
	MsgLedgerExportTitle                = "ledger_export.title"
	MsgLedgerExportPeriod               = "ledger_export.period"
	MsgLedgerExportTimezone             = "ledger_export.timezone"
	MsgLedgerExportScope                = "ledger_export.scope"
	MsgLedgerExportKindFilter           = "ledger_export.kind_filter"
	MsgLedgerExportKindFilterAll        = "ledger_export.kind_filter_all"
	MsgLedgerExportNote                 = "ledger_export.note"
	MsgLedgerExportScopeCustomer        = "ledger_export.scope_customer"
	MsgLedgerExportScopeReseller        = "ledger_export.scope_reseller"
	MsgLedgerExportScopeAdminRollup     = "ledger_export.scope_admin_rollup"
	MsgLedgerExportSectionSummary       = "ledger_export.section_summary"
	MsgLedgerExportSectionByKind        = "ledger_export.section_by_kind"
	MsgLedgerExportSectionRows          = "ledger_export.section_rows"
	MsgLedgerExportCreditIncrease       = "ledger_export.credit_increase"
	MsgLedgerExportCreditDecrease       = "ledger_export.credit_decrease"
	MsgLedgerExportCreditNet            = "ledger_export.credit_net"
	MsgLedgerExportActualUsage          = "ledger_export.actual_usage"
	MsgLedgerExportFeeCharge            = "ledger_export.fee_charge"
	MsgLedgerExportFeeCost              = "ledger_export.fee_cost"
	MsgLedgerExportFeeProfit            = "ledger_export.fee_profit"
	MsgLedgerExportRedemptionIssued     = "ledger_export.redemption_issued"
	MsgLedgerExportRedemptionRefunded   = "ledger_export.redemption_refunded"
	MsgLedgerExportRedemptionEscrowNet  = "ledger_export.redemption_escrow_net"
	MsgLedgerExportColItem              = "ledger_export.col_item"
	MsgLedgerExportColCount             = "ledger_export.col_count"
	MsgLedgerExportColQuota             = "ledger_export.col_quota"
	MsgLedgerExportColAmountUsd         = "ledger_export.col_amount_usd"
	MsgLedgerExportColPaidUsd           = "ledger_export.col_paid_usd"
	MsgLedgerExportColKind              = "ledger_export.col_kind"
	MsgLedgerExportColDate              = "ledger_export.col_date"
	MsgLedgerExportColCustomer          = "ledger_export.col_customer"
	MsgLedgerExportColReseller          = "ledger_export.col_reseller"
	MsgLedgerExportColOperator          = "ledger_export.col_operator"
	MsgLedgerExportColRef               = "ledger_export.col_ref"
	MsgLedgerExportColRemark            = "ledger_export.col_remark"
	MsgLedgerExportNoRows               = "ledger_export.no_rows"
	MsgLedgerExportKindAdminCredit      = "ledger_export.kind_admin_credit"
	MsgLedgerExportKindAdminDebit       = "ledger_export.kind_admin_debit"
	MsgLedgerExportKindCustomerCredit   = "ledger_export.kind_customer_credit"
	MsgLedgerExportKindCustomerDebit    = "ledger_export.kind_customer_debit"
	MsgLedgerExportKindRedemptionIssue  = "ledger_export.kind_redemption_issue"
	MsgLedgerExportKindRedemptionRefund = "ledger_export.kind_redemption_refund"
	MsgLedgerExportKindRedemptionRedeem = "ledger_export.kind_redemption_redeem"
)
