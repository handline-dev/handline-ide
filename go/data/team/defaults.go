package team

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// DefaultsVendors are the vendors whose new agents take a model and effort from config.yaml (DefaultModelKey, DefaultEffortKey).
var DefaultsVendors = []string{"claude", "codex"}

// DefaultModelKey and DefaultEffortKey are config.yaml's names for the model and effort a new agent of vendor gets when it is registered
// with none: default_model_claude, default_effort_codex and so on.
func DefaultModelKey(vendor string) string  { return "default_model_" + vendor }
func DefaultEffortKey(vendor string) string { return "default_effort_" + vendor }

// ModelMax is how long a model name may be, in characters.
const ModelMax = 100

// ValidModelName is whether s can be handed to a vendor's CLI as its model: one word, not an option.
func ValidModelName(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !textsafe.HasUnsafe(s) && !strings.ContainsAny(s, " '\"") &&
		utf8.RuneCountInString(s) <= ModelMax
}

// builtinModel is vendor's model when nothing sets one: sonnet for claude; "" for any other, which its CLI reads as its own default.
func builtinModel(vendor string) string {
	if vendor == DefaultVendor {
		return DefaultModel
	}
	return ""
}

// NewAgentDefaultsOf is the model and effort a new agent of vendor gets under cfg, config.yaml as read: its default_model_<vendor> and
// default_effort_<vendor>, else the built-in ones.
func NewAgentDefaultsOf(cfg map[string]any, vendor string) (model, effort string) {
	model, effort = builtinModel(vendor), DefaultEffort
	if m, ok := cfg[DefaultModelKey(vendor)].(string); ok && ValidModelName(m) {
		model = m
	}
	if e, ok := cfg[DefaultEffortKey(vendor)].(string); ok && contains(ValidEffort, e) {
		effort = e
	}
	return model, effort
}

// NewAgentDefaults is NewAgentDefaultsOf home's config.yaml; one that cannot be read sets nothing.
func NewAgentDefaults(home, vendor string) (model, effort string) {
	var cfg map[string]any
	if b, err := os.ReadFile(paths.ConfigFile(home)); err == nil {
		if v, err := yamlcompat.Parse(string(b)); err == nil {
			cfg, _ = v.(map[string]any)
		}
	}
	return NewAgentDefaultsOf(cfg, vendor)
}
