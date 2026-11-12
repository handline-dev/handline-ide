package adminwrites

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// account.add and account.remove (handline/team/accounts.py add_account, remove_account): the account's directory made and linked to the
// shared files with nothing logged in, and the sequence of `hl account remove`. The login is account.login's; trusting the agents'
// worktrees on the new account is settings.trust's.

func (e *Env) accountAdd(c *rpc.Conn, p map[string]any) (any, error) {
	vendor := ""
	if v := p["vendor"]; v != nil {
		var ok bool
		if vendor, ok = v.(string); !ok || vendor == "" {
			return nil, badParam("vendor is the tool the account is for", "vendor")
		}
	}
	return e.addAccount(c, p, vendor, nil, func(acct string) string {
		return fmt.Sprintf("registered account '%s'; log it in from the app's Accounts page", acct)
	})
}

// accountsDiscover is accounts.discover: the logins already on this Mac an account could use.
func (e *Env) accountsDiscover(*rpc.Conn, map[string]any) (any, error) {
	found := []any{}
	for _, f := range team.Discover() {
		var in any
		if f.LoggedIn != nil {
			in = *f.LoggedIn
		}
		found = append(found, map[string]any{"vendor": f.Vendor, "source": f.Source, "logged_in": in, "adoptable": f.Adoptable})
	}
	return map[string]any{"found": found}, nil
}

// accountAdopt is account.adopt: an account that uses a codex login accounts.discover found.
func (e *Env) accountAdopt(c *rpc.Conn, p map[string]any) (any, error) {
	vendor, _ := p["vendor"].(string)
	source, ok := p["source"].(string)
	if !ok || source == "" {
		return nil, badParam("source is a login accounts.discover found", "source")
	}
	link, err := team.AdoptLink(vendor, source)
	switch {
	case errors.Is(err, team.ErrNotAdoptable):
		return nil, badParam(err.Error(), "vendor")
	case errors.Is(err, team.ErrNoSource):
		return nil, notFound("source", source, err.Error())
	case errors.Is(err, team.ErrSourceLoggedOut):
		return nil, refused(err.Error(), "state")
	case err != nil:
		return nil, err
	}
	return e.addAccount(c, p, vendor, []team.Link{link}, func(acct string) string {
		return fmt.Sprintf("registered account '%s', logged in as %s", acct, source)
	})
}

// addAccount adds the account the params name, or account<n> when they name none, moving past a name in use, with links in its config
// dir; said is the reply for the name.
func (e *Env) addAccount(c *rpc.Conn, p map[string]any, vendor string, links []team.Link, said func(acct string) string) (any, error) {
	acct, named := "", p["account"] != nil
	if named {
		var ok bool
		if acct, ok = p["account"].(string); !ok || acct == "" {
			return nil, badParam("account is the new account's name", "account")
		}
	}
	accts := team.Accounts{Home: e.Home}
	var err error
	if named {
		_, err = accts.Add(acct, vendor, links...)
	} else {
		for n := len(paths.ListAccounts(e.Home)) + 1; ; n++ {
			acct = fmt.Sprintf("account%d", n)
			if _, err = accts.Add(acct, vendor, links...); !errors.Is(err, team.ErrAccountExists) {
				break
			}
		}
	}
	var unknown *team.UnknownVendor
	switch {
	case errors.Is(err, team.ErrBadAccountName):
		return nil, badParam(err.Error(), "account")
	case errors.As(err, &unknown):
		return nil, badParam(err.Error(), "vendor")
	case errors.Is(err, team.ErrAccountExists):
		return nil, refused(err.Error(), "state")
	case err != nil:
		return nil, refused(err.Error(), "")
	}
	out, err := wrote(c, said(acct), map[string]any{"account": acct})
	if err == nil && e.Changed != nil {
		e.Changed(acct)
	}
	return out, err
}

func (e *Env) accountRemove(c *rpc.Conn, p map[string]any) (any, error) {
	acct, err := e.accountNamed(p)
	if err != nil {
		return nil, err
	}
	dry, _, err := flagParam(p, "dry_run")
	if err != nil {
		return nil, err
	}
	confirm := ""
	if v := p["confirm"]; v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, badParam("confirm is the account's name", "confirm")
		}
		confirm = s
	}
	accts := team.Accounts{Home: e.Home}
	if dry {
		would, lines, err := accts.RemovePreview(e.DB, acct)
		if err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true, "said": strings.Join(lines, "\n"), "would_remove": would, "plan": lines}, nil
	}
	if strings.TrimSpace(confirm) != acct {
		return nil, badParam(fmt.Sprintf("confirm is the account's name, %q, as the founder typed it", acct), "confirm")
	}
	run := e.Run
	if run == nil {
		run = logout
	}
	said, err := accts.Remove(e.DB, acct, time.Now(), run)
	var no *team.RemoveRefused
	if errors.As(err, &no) {
		return nil, refused(no.Why, "state")
	}
	if err != nil {
		return nil, err
	}
	if rid, ok := p["request_id"].(string); ok {
		if err := c.RecordWrite("account.remove", rid); err != nil {
			return nil, err
		}
	}
	return map[string]any{"accepted": true, "said": said}, nil
}

// logout runs the vendor's logout command with the account's environment. The vendor removes its own credential.
func logout(argv []string, env map[string]string) int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := osexec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = paths.QuietDir()
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := cmd.Run(); err != nil {
		var exit *osexec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		return -1
	}
	return 0
}
