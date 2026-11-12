package settings

import (
	"errors"
	"fmt"
	"os"

	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/atomicfile"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// Never is a park_timeout_s of 0, global or per agent: the explicit "never close this window" override (a real answer, not an absent key).
const Never = 0

// ClearPastBackgroundKey lets a context clear go ahead once the turn has ended with only background work running. Off by default.
const ClearPastBackgroundKey = "context_clear_past_background"

// TranscriptRowsKey stores a running turn's replies, tool calls and results for chat (transcript.Record). Off by default.
const TranscriptRowsKey = "chat_transcript_rows"

// RefsNeedGrantKey refuses chat.send when a ref is outside what the agent may read (contract x-access). Off by default.
const RefsNeedGrantKey = "chat_refs_need_grant"

const (
	// BgGraceS: an agent still running background work when its idle window is over is told to finish or stop it, and is stopped this
	// long after that if it shows nothing more.
	BgGraceS = 300
	// BgCheckS is how often an agent with background work is asked which of it to stop.
	BgCheckS = 600
	// NoticeDigestS: notices to an agent are held this long after the first, then typed once as a digest at its prompt.
	NoticeDigestS = 300
)

// The thresholds of RequestRow.attention: a tree task waiting on cap, wall or owner this long, an open tree this long with no activity,
// a tree task sent back (reopened) this many times.
const (
	AttentionWaitKey     = "attention_wait_s"
	AttentionWaitS       = 3600
	AttentionQuietKey    = "attention_quiet_s"
	AttentionQuietS      = 7200
	AttentionSentBackKey = "attention_sent_back"
	AttentionSentBack    = 2
)

// The blocker alarm: a queued task another open task has waited on this long tells its holder, and this long the task's opener. Off by default.
const (
	BlockerAlarmKey       = "blocker_alarm"
	BlockerAlarmHoldKey   = "blocker_alarm_s"
	BlockerAlarmHoldS     = 3600
	BlockerAlarmOpenerKey = "blocker_alarm_opener_s"
	BlockerAlarmOpenerS   = 4 * 3600
)

// BlockerAlarm is whether the blocker alarm is on and its two thresholds.
func (s *Settings) BlockerAlarm() (on bool, holderS, openerS int64) {
	return s.Flag(BlockerAlarmKey, false), s.ConfigSeconds(BlockerAlarmHoldKey, BlockerAlarmHoldS), s.ConfigSeconds(BlockerAlarmOpenerKey, BlockerAlarmOpenerS)
}

// Settings is how long an idle agent's session may sit before it is closed, the background times and the cache-expiry switch: config.yaml's
// global values, and its row's own override for the one agent that should behave differently.
type Settings struct {
	Home   string
	Roster *team.Roster
}

func positive(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), n > 0
	case int64:
		return n, n > 0
	}
	return 0, false
}

func nonNegative(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), n >= 0
	case int64:
		return n, n >= 0
	}
	return 0, false
}

// config is config.yaml as the readers see it; one that cannot be read is empty.
func (s *Settings) config() map[string]any {
	b, err := os.ReadFile(paths.ConfigFile(s.Home))
	if err != nil {
		return map[string]any{}
	}
	v, err := yamlcompat.Parse(string(b))
	if err != nil {
		return map[string]any{}
	}
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

// agentConfig is the agent's own config, {} for one that cannot be read: closing must not stall on a bad agent row.
func (s *Settings) agentConfig(agent string) map[string]any {
	if s.Roster == nil {
		return map[string]any{}
	}
	cfg, err := s.Roster.Read(agent)
	if err != nil || cfg == nil {
		return map[string]any{}
	}
	return cfg
}

// ConfigSeconds is a positive number of seconds from config.yaml, or def when it is absent, not an integer or not above zero.
func (s *Settings) ConfigSeconds(key string, def int64) int64 {
	if n, ok := positive(s.config()[key]); ok {
		return n
	}
	return def
}

// Percent is a whole percent from 1 to 100 from config.yaml, or 0 when it is absent or not one.
func (s *Settings) Percent(key string) int {
	if n, ok := positive(s.config()[key]); ok && n <= 100 {
		return int(n)
	}
	return 0
}

// Flag is a true/false setting from config.yaml, or def when it is absent or not one.
func (s *Settings) Flag(key string, def bool) bool {
	if b, ok := s.config()[key].(bool); ok {
		return b
	}
	return def
}

// SetFlag writes one global setting, keeping every other. A config.yaml that cannot be read as a map is not overwritten.
func (s *Settings) SetFlag(key string, value bool) error {
	path := paths.ConfigFile(s.Home)
	var pairs []yamlcompat.Pair
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("%s: %w", path, err)
	default:
		var ok bool
		pairs, ok, err = yamlcompat.ParsePairs(string(b))
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if !ok {
			if len(bytesTrim(b)) != 0 {
				return fmt.Errorf("%s is not a mapping; not overwriting it", path)
			}
			pairs = nil
		}
	}
	set := false
	for i := range pairs {
		if pairs[i].Key == key {
			pairs[i].Val, set = value, true
		}
	}
	if !set {
		pairs = append(pairs, yamlcompat.Pair{Key: key, Val: value})
	}
	text, err := yamlcompat.Dump(pairs)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, text)
}

func bytesTrim(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\t' || b[j-1] == '\r') {
		j--
	}
	return b[i:j]
}

// DefaultCloseTimeoutS is the park timeout every agent without its own override follows.
func (s *Settings) DefaultCloseTimeoutS() int64 {
	if n, ok := nonNegative(s.config()["park_timeout_s"]); ok {
		return n
	}
	return states.IdleCloseS
}

// CloseTimeoutS is the seconds this agent may sit idle before the daemon closes it; Never (0) means it never does. The agent's own
// park_timeout_s wins when it is set, then config.yaml's global one, then the measured default.
func (s *Settings) CloseTimeoutS(agent string) int64 {
	if n, ok := nonNegative(s.agentConfig(agent)["park_timeout_s"]); ok {
		return n
	}
	return s.DefaultCloseTimeoutS()
}

// Never is Never, for a port that cannot name the constant.
func (s *Settings) Never() int64 { return Never }

// SecondsSetting is a positive number of seconds: the agent's own column wins, then config.yaml's, then def.
func (s *Settings) SecondsSetting(agent, key string, def int64) int64 {
	if n, ok := positive(s.agentConfig(agent)[key]); ok {
		return n
	}
	if n, ok := positive(s.config()[key]); ok {
		return n
	}
	return def
}

// TextSetting is a text setting: the agent's own when it sets one that is not empty, then config.yaml's, "" for neither.
func (s *Settings) TextSetting(agent, key string) string {
	if t, ok := s.agentConfig(agent)[key].(string); ok && t != "" {
		return t
	}
	t, _ := s.config()[key].(string)
	return t
}

// BgNoticeS is how long an agent with background work may show nothing before it is told to finish or stop it: its idle-close window
// unless bg_notice_s says otherwise.
func (s *Settings) BgNoticeS(agent string) int64 {
	return s.SecondsSetting(agent, "bg_notice_s", s.CloseTimeoutS(agent))
}

// BgGraceS is how long after that notice its work is stopped if it still shows nothing.
func (s *Settings) BgGraceS(agent string) int64 {
	return s.SecondsSetting(agent, "bg_grace_s", BgGraceS)
}

// BgCheckS is how often, while it has background work, an agent is asked which of it to stop.
func (s *Settings) BgCheckS(agent string) int64 {
	return s.SecondsSetting(agent, "bg_check_s", BgCheckS)
}

// NoticeDigestS is how long after its first waiting notice the agent is typed one digest, at its prompt.
func (s *Settings) NoticeDigestS(agent string) int64 {
	return s.SecondsSetting(agent, "notice_digest_s", NoticeDigestS)
}

// IgnoreCacheExpiry is whether this agent's resume may reuse its session past the prompt cache's expiry instead of starting fresh: its
// own ignore_cache_expiry wins when set, then config.yaml's global one, off otherwise.
func (s *Settings) IgnoreCacheExpiry(agent string) bool {
	if b, ok := s.agentConfig(agent)["ignore_cache_expiry"].(bool); ok {
		return b
	}
	return s.config()["ignore_cache_expiry"] == true
}
