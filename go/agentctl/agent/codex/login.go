package codex

// WebLoginArgv: no web login yet (V2 adds codex login --device-auth).
func (Account) WebLoginArgv() ([]string, bool) { return nil, false }

// CredentialPresent is whether auth.json is there (V2 asks codex login status).
func (a Account) CredentialPresent(configDir string) (present, known bool) {
	return a.CredentialModified(configDir) != nil, true
}
