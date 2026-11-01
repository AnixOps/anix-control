// Package settingscatalog declares the settings the console offers as
// options (系统设置): which section each belongs to, its kind, its choices
// and its limits. The console draws its options from this list instead of
// asking the administrator to type a configuration key; a key that is not
// here is reachable only in the raw table under 高级.
package settingscatalog

import (
	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// Sections of 系统设置 that hold options.
const (
	SectionGeneral = "general"
	SectionData    = "data"
)

// Kinds of option.
const (
	// KindDomains is a list of host names, edited as text.
	KindDomains = "domains"
	// KindSwitch is on or off.
	KindSwitch = "switch"
	// KindChoice is one of Choices (whole numbers).
	KindChoice = "choice"
	// KindLink is a setting that lives on its own page (Link).
	KindLink = "link"
)

// Item is one option.
type Item struct {
	// Key is the system configuration key the option stores in; a link
	// names the key of the setting it opens.
	Key     string `json:"key"`
	Section string `json:"section"`
	Kind    string `json:"kind"`
	// Choices are the values a KindChoice option offers, ascending.
	Choices []int `json:"choices,omitempty"`
	// Min and Max bound a KindChoice value the API accepts.
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
	// Default is the value in force while the key is unset.
	Default any `json:"default,omitempty"`
	// Value is the value in force now; absent for a kind the console reads
	// elsewhere (domains, links).
	Value any `json:"value,omitempty"`
	// Link is the console route of a KindLink option.
	Link string `json:"link,omitempty"`
	// Sensitive marks an option whose value is never answered.
	Sensitive bool `json:"sensitive,omitempty"`
}

func intp(v int) *int { return &v }

// Items returns the catalog, with the values in force now read from db.
// A nil db leaves the values out.
func Items(db *gorm.DB) ([]Item, error) {
	retention := Item{
		Key: destinationstats.RetentionDaysKey, Section: SectionData, Kind: KindChoice,
		Choices: []int{7, 30, 60, 90, 180},
		Min:     intp(destinationstats.MinRetentionDays), Max: intp(destinationstats.MaxRetentionDays),
		Default: destinationstats.DefaultRetentionDays,
	}
	enabled := Item{Key: destinationstats.EnabledKey, Section: SectionData, Kind: KindSwitch, Default: false}
	domainsOnly := Item{Key: service.SystemConfigKeySubscribeDomainsOnly, Section: SectionGeneral, Kind: KindSwitch, Default: false}
	if db != nil {
		domainsOnly.Value = service.GetSubscriptionSettings(service.NewSystemConfigService(db), nil).DomainsOnly
		settings, err := destinationstats.LoadSettings(db)
		if err != nil {
			return nil, err
		}
		retention.Value = settings.RetentionDays
		enabled.Value = settings.Enabled
	}
	items := []Item{
		{Key: service.SystemConfigKeySubscribeDomains, Section: SectionGeneral, Kind: KindDomains},
		domainsOnly,
		{Key: "notification.email.config", Section: SectionGeneral, Kind: KindLink, Link: "/admin/notifications/email"},
		{Key: "security.mfa.config", Section: SectionGeneral, Kind: KindLink, Link: "/admin/security/mfa"},
		enabled,
		retention,
	}
	for i := range items {
		items[i].Sensitive = service.IsSensitiveSystemConfigKey(items[i].Key)
	}
	return items, nil
}
