package sessionrpc

import (
	"github.com/handline/handline/go/protocol/rpc"
)

// LaunchRequest is what a terminal's `hl launch` knows that the daemon does not: where it runs and what it was given.
type LaunchRequest struct {
	Agent, Account, Why, By, PromptFile string
	Extra                               []string
	Force                               bool
	Cwd, Binary, HlExe                  string
	Env                                 map[string]string
	Handle, WorktreeID                  string
	PID                                 int
}

// LaunchResult is the command line and environment the caller becomes, and what it should tell the person first.
type LaunchResult struct {
	Argv, Env, Warnings []string
}

// LaunchRefusal is a launch that must not go ahead, in words for the person at the terminal.
type LaunchRefusal struct{ Msg string }

func (r *LaunchRefusal) Error() string { return r.Msg }

// launchPrepare is session.launch_prepare: the builds and records of `hl launch`, which then locks the worktree and execs what this
// returns. Only the local socket serves it, for a process that is not yet any agent's session; the pid is the peer's own.
func (e *Env) launchPrepare(c *rpc.Conn, params map[string]any) (any, error) {
	if c.Server().Transport != "local" || c.Pid <= 0 {
		return nil, rpc.Refused("a launch is prepared on the local socket only", "role", "")
	}
	var err error
	str := func(k string) (string, error) {
		v, present := params[k]
		if !present {
			return "", nil
		}
		s, ok := v.(string)
		if !ok {
			return "", bad(k+" is text", k)
		}
		return s, nil
	}
	req := LaunchRequest{PID: c.Pid}
	for k, dst := range map[string]*string{"account": &req.Account, "why": &req.Why, "by": &req.By, "prompt_file": &req.PromptFile,
		"cwd": &req.Cwd, "binary": &req.Binary, "hl_exe": &req.HlExe, "handle": &req.Handle, "worktree_id": &req.WorktreeID} {
		if *dst, err = str(k); err != nil {
			return nil, err
		}
	}
	if req.Cwd == "" || req.Cwd[0] != '/' {
		return nil, bad("cwd is the absolute folder the launch runs in", "cwd")
	}
	if req.Force, err = flag(params, "force"); err != nil {
		return nil, err
	}
	if list, present := params["extra"]; present {
		items, ok := list.([]any)
		if !ok {
			return nil, bad("extra is a list of text", "extra")
		}
		for _, it := range items {
			t, ok := it.(string)
			if !ok {
				return nil, bad("extra is a list of text", "extra")
			}
			req.Extra = append(req.Extra, t)
		}
	}
	req.Env = map[string]string{}
	if m, present := params["env"]; present {
		obj, ok := m.(map[string]any)
		if !ok {
			return nil, bad("env is an object of text", "env")
		}
		for k, v := range obj {
			t, ok := v.(string)
			if !ok {
				return nil, bad("env is an object of text", "env")
			}
			req.Env[k] = t
		}
	}
	if req.Agent, err = e.agent(params); err != nil { // the roster is asked once the input is known to be well formed
		return nil, err
	}
	res, err := e.Launch(req)
	if refused, ok := err.(*LaunchRefusal); ok {
		return nil, rpc.Refused(refused.Msg, "state", "")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"argv": res.Argv, "env": res.Env, "warnings": res.Warnings}, nil
}
