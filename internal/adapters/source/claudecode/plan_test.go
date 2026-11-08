package claudecode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanName(t *testing.T) {
	for _, c := range []struct{ org, tier, want string }{
		{"claude_max", "default_claude_max_5x", "Max 5×"},
		{"claude_max", "default_claude_max_20x", "Max 20×"},
		{"claude_pro", "", "Pro"},
		{"claude_team", "default_raven", "Team"},
		{"", "default_claude_max_5x", ""},
	} {
		if got := planName(c.org, c.tier); got != c.want {
			t.Errorf("planName(%q, %q) = %q, want %q", c.org, c.tier, got, c.want)
		}
	}
}

func TestPlanReadsOnlyThePlan(t *testing.T) {
	dir := t.TempDir()
	r := Reader{Home: t.TempDir(), ConfigDir: dir}
	if p := r.Plan(); p != (Plan{}) {
		t.Errorf("no account file: got %+v", p)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"a@b.c",`+
		`"organizationType":"claude_max","billingType":"stripe_subscription","organizationRateLimitTier":"default_claude_max_5x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := r.Plan(); p != (Plan{Name: "Max 5×", Subscription: true}) {
		t.Errorf("Max plan: got %+v", p)
	}
	for _, cfg := range []string{"", "/h/.claude", "/h/.claude/"} {
		if got := (Reader{Home: "/h", ConfigDir: cfg}).AccountFile(); got != "/h/.claude.json" {
			t.Errorf("default account file with CLAUDE_CONFIG_DIR %q: got %s", cfg, got)
		}
	}
}
