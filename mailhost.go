package warmbly

// Who hosts an inbox, in [Contact.MailHost], [Email.MailHost] and
// [ContactSearchParams.MailHosts]. A contact's value is read from its domain's
// MX records; a mailbox's is recorded when it connects. The set grows, so a
// value not listed here still decodes: treat it like [MailHostOther] rather than
// failing. Empty means not detected yet.
const (
	MailHostGoogleWorkspace = "google_workspace"
	MailHostGmail           = "gmail"
	MailHostMicrosoft365    = "microsoft365"
	MailHostOutlook         = "outlook"
	MailHostZoho            = "zoho"
	MailHostYahoo           = "yahoo"
	MailHostAOL             = "aol"
	MailHostICloud          = "icloud"
	MailHostFastmail        = "fastmail"
	MailHostGoDaddy         = "godaddy"
	MailHostNamecheap       = "namecheap"
	MailHostIONOS           = "ionos"
	MailHostHostinger       = "hostinger"
	MailHostOVH             = "ovh"
	MailHostMigadu          = "migadu"
	MailHostPurelymail      = "purelymail"
	MailHostRackspace       = "rackspace"
	MailHostYandex          = "yandex"
	MailHostGMX             = "gmx"
	MailHostProton          = "proton"
	// MailHostOther is a checked domain with no known host.
	MailHostOther = "other"
)
