package db

import (
	"encoding/json"
	"slices"
)

// SettingsOwed is a restart for changed settings that waits for the agent's turn to end: the settings, in launch order, and since when.
type SettingsOwed struct {
	Settings []string `json:"settings"`
	Since    int64    `json:"since"`
}

func settingsOwedMeta(agent string) string { return "settings_applied:" + agent + ":owed" }

// SetSettingsOwed keeps that agent owes a restart for these settings, from since; the same settings keep their first since, and none
// clears it.
func SetSettingsOwed(q Q, agent string, settings []string, since int64) error {
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
		return nil
	}
	b, err := json.Marshal(SettingsOwed{Settings: settings, Since: since})
	if err != nil {
		return err
	}
	return SetMeta(q, settingsOwedMeta(agent), string(b))
}

// GetSettingsOwed is the restart agent owes for changed settings, nil for none.
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
