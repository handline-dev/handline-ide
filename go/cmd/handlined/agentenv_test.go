package main

import (
	"os"
	"testing"
)

func TestADaemonStartedFromAnAgentsSessionDropsItsAgentVariables(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "a5")
	t.Setenv("HANDLINE_ACCOUNT", "alpha")
	if got := leaveAgentSession(); got != "a5" {
		t.Fatalf("names the agent: %q", got)
	}
	for _, k := range []string{"HANDLINE_AGENT", "HANDLINE_ACCOUNT"} {
		if _, set := os.LookupEnv(k); set {
			t.Errorf("%s is still set", k)
		}
	}
	if got := leaveAgentSession(); got != "" {
		t.Fatalf("none left: %q", got)
	}
}
