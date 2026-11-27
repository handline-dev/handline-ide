package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/taskboard/watch"
)

// originRepo is the repository of the origin of the checkout hl runs in, as its pull requests are named (owner/name on GitHub,
// host/path elsewhere); "" when there is none.
func originRepo() string {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return watch.OriginRepo(string(out))
}

// taskWait is hl task wait T<n> (--on <vendor>:<target> | --pr <n> [--repo owner/name]) [--until <what>] [--then done] [-t <span>]: one
// task.wait, which opens a gate on the target, blocks T<n> on it and, when doing, releases it, so its holder takes other work and T<n>
// comes back to its queue when the gate opens. --pr is github:<origin repo>#<n> (gitlab:<host/path>!<n> off GitHub); --until defaults to the vendor's first (a PR's CI).
// --then done, with --until landed, has the merge close T<n> with its commit in place of bringing it back.
func taskWait(c *hlclient.Client, args []string) (int, bool) {
	p, ok := parse(args, []string{"--on", "--pr", "--repo", "--until", "--then", "--timeout"}, nil, map[string]string{"-t": "--timeout"})
	if !ok || len(p.pos) != 1 {
		return 0, false
	}
	tid, ok := taskRef(p.pos[0])
	secs, hasSecs, ok2 := optTimeout(p, "--timeout")
	on, hasOn := p.opt("--on")
	prText, hasPR := p.opt("--pr")
	repo, hasRepo := p.opt("--repo")
	if !ok || !ok2 || hasOn == hasPR || hasRepo && !hasPR {
		return 0, false
	}
	vendor, target, _ := strings.Cut(on, ":")
	if hasPR {
		pr, err := strconv.ParseInt(strings.TrimPrefix(prText, "#"), 10, 64)
		if err != nil || pr <= 0 {
			return 0, false
		}
		if !hasRepo {
			if repo = originRepo(); repo == "" {
				fmt.Println("hl task wait: this checkout has no origin; name the repo with --repo owner/name, or the target with --on")
				return 1, true
			}
		}
		vendor, target = watch.PullTarget(repo, pr)
	}
	mutating(c, "wait")
	if !asking(c, "make a task wait") {
		return 1, true
	}
	w := map[string]any{"vendor": vendor, "target": target}
	if until, given := p.opt("--until"); given {
		w["until"] = until
	}
	params := map[string]any{"task": tid, "watch": w}
	if then, given := p.opt("--then"); given {
		params["then"] = then // the daemon says what it takes
	}
	if hasSecs {
		params["time_box"] = fmt.Sprintf("%ds", secs)
	}
	ok, words := c.Write("task.wait", params)
	return said(ok, words), true
}
