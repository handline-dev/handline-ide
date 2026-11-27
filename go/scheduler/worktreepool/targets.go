package worktreepool

// The founder's machine targets in config.yaml, in GB; 0, the default, is off. Over AgentsMemoryMaxKey of the agents' memory the longest-idle sessions close.
const (
	DiskKeepFreeKey    = "disk_keep_free_gb"
	AgentsMemoryMaxKey = "agents_memory_max_gb"
)

// GB is one of the targets in bytes: 0 when it is off, unset or not a whole number above 0.
func GB(cfg map[string]any, key string) int64 {
	switch n := cfg[key].(type) {
	case int:
		return max(int64(n), 0) << 30
	case int64:
		return max(n, 0) << 30
	}
	return 0
}
