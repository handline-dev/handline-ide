package db

// SettingsByMeta is the meta name that keeps who made agent's newest settings change (agent.set), for the restart that applies it to
// say who asked; "" or none says the daemon.
func SettingsByMeta(agent string) string { return "settings_applied:" + agent + ":by" }
