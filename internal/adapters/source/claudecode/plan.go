package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Plan is the Claude plan the logs' account is on, as far as Claude Code's
// account file says (ADR 0048). Only these fields are read from that file.
type Plan struct {
	Name         string // for people: "Max 5×", "Pro"; "" if not known
	Subscription bool   // billed as a subscription (a flat fee), not per token
}

// account is the part of Claude Code's account file Plan reads.
type account struct {
	OAuthAccount *struct {
		OrganizationType          string `json:"organizationType"`          // e.g. "claude_max"
		BillingType               string `json:"billingType"`               // e.g. "stripe_subscription"
		OrganizationRateLimitTier string `json:"organizationRateLimitTier"` // e.g. "default_claude_max_5x"
		UserRateLimitTier         string `json:"userRateLimitTier"`
	} `json:"oauthAccount"`
}

// AccountFile is Claude Code's account file: .claude.json in the
// configuration folder when it was moved, else in the home folder (also
// when CLAUDE_CONFIG_DIR names the default folder).
func (r Reader) AccountFile() string {
	if r.ConfigDir != "" && filepath.Clean(r.ConfigDir) != filepath.Join(r.Home, ".claude") {
		return filepath.Join(r.ConfigDir, ".claude.json")
	}
	return filepath.Join(r.Home, ".claude.json")
}

// Plan reads the plan from the account file. A missing or unreadable file,
// or one signed in with an API key, gives an empty Plan.
func (r Reader) Plan() Plan {
	b, err := os.ReadFile(r.AccountFile())
	if err != nil {
		return Plan{}
	}
	var a account
	if json.Unmarshal(b, &a) != nil || a.OAuthAccount == nil {
		return Plan{}
	}
	o := a.OAuthAccount
	tier := o.UserRateLimitTier
	if tier == "" {
		tier = o.OrganizationRateLimitTier
	}
	return Plan{
		Name:         planName(o.OrganizationType, tier),
		Subscription: strings.Contains(o.BillingType, "subscription"),
	}
}

var tierMultiple = regexp.MustCompile(`_(\d+)x$`)

// planName names a plan for people: "claude_max" with tier
// "default_claude_max_5x" is "Max 5×".
func planName(orgType, tier string) string {
	name := strings.TrimPrefix(orgType, "claude_")
	if name == "" {
		return ""
	}
	name = strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "_", " ")
	if m := tierMultiple.FindStringSubmatch(tier); m != nil {
		name += " " + m[1] + "×"
	}
	return name
}

// DefaultRetentionDays is how long Claude Code keeps session logs when
// cleanupPeriodDays is not set.
const DefaultRetentionDays = 30

// Retention reads how many days Claude Code keeps session logs: the
// cleanupPeriodDays key of its settings file, and only that key (ADR 0050).
// set is false when the key is missing, and days is then the default.
func (r Reader) Retention() (days int, set bool) {
	dir := r.ConfigDir
	if dir == "" {
		dir = filepath.Join(r.Home, ".claude")
	}
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return DefaultRetentionDays, false
	}
	var s struct {
		// A number, which may be written 365.0.
		CleanupPeriodDays *float64 `json:"cleanupPeriodDays"`
	}
	if json.Unmarshal(b, &s) != nil || s.CleanupPeriodDays == nil {
		return DefaultRetentionDays, false
	}
	return int(*s.CleanupPeriodDays), true
}
