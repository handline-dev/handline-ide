package db

import (
	"encoding/json"
	"slices"
)

// SettingsOwed is what the agent's running session lacks of its changed settings: the settings, in launch order, and since when. Restart
// is a restart that waits for the agent's turn to end to give them; without it they wait for the session's next fresh start.
type SettingsOwed struct {
	Settings []string `json:"settings"`
	Since    int64    `json:"since"`
	Restart  bool     `json:"restart,omitempty"`
}

func settingsOwedMeta(agent string) string { return "settings_applied:" + agent + ":owed" }

// SetSettingsOwed keeps that agent owes these settings, from since, by a restart or a fresh start; the same settings keep their first
// since, and none clears it.
func SetSettingsOwed(q Q, agent string, settings []string, since int64, restart bool) error {
	was, err := GetSettingsOwed(q, agent)
	if err != nil {
		return err
	}
	if len(settings) == 0 {
		if was == nil {
			return nil
		}
		return SetMeta(q, settingsOwedMeta(agent), "")
	}
	if was != nil && slices.Equal(was.Settings, settings) {
		if was.Restart == restart {
			return nil
		}
		since = was.Since
	}
	b, err := json.Marshal(SettingsOwed{Settings: settings, Since: since, Restart: restart})
	if err != nil {
		return err
	}
	return SetMeta(q, settingsOwedMeta(agent), string(b))
}

// GetSettingsOwed is what agent owes of its changed settings, nil for none.
func GetSettingsOwed(q Q, agent string) (*SettingsOwed, error) {
	v, _, err := Meta(q, settingsOwedMeta(agent))
	if err != nil || v == "" {
		return nil, err
	}
	var o SettingsOwed
	if json.Unmarshal([]byte(v), &o) != nil || len(o.Settings) == 0 {
		return nil, nil
	}
	return &o, nil
}
