package reads

import (
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
)

func TestTheSettingsPageReadsTheHooksTheLaunchInstalls(t *testing.T) {
	t.Parallel()
	a, b := slices.Sorted(slices.Values(hooksWanted)), slices.Sorted(slices.Values(claudecode.Wanted))
	if !slices.Equal(a, b) {
		t.Fatalf("settings reads %v, the launch installs %v", hooksWanted, claudecode.Wanted)
	}
}
