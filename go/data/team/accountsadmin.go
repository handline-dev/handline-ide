package team

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// Why an account was not added.
var (
	ErrBadAccountName = errors.New("bad account name")
	ErrAccountExists  = errors.New("account exists")
)

// Add registers an account and links it to the shared files, as `hl account add --no-login`: its directory is made and nothing is logged
// in. Everything is checked before anything is made, and a failure while linking takes the new directory away again, so a refusal leaves
// no account behind. A name that is empty, holds a slash, starts with a dot or would not read back as text is ErrBadAccountName (wrapped,
// with the reason); a vendor with no driver is *UnknownVendor; a name in use is ErrAccountExists. Returns the config directory made.
func (a Accounts) Add(acct, vendor string) (string, error) {
	if acct == "" || strings.Contains(acct, "/") || strings.HasPrefix(acct, ".") {
		return "", fmt.Errorf("%w: %q", ErrBadAccountName, acct)
	}
	if !yamlcompat.IsPlainText(acct) {
		return "", fmt.Errorf("%w: %q. It would not be read back as text from the YAML files it is written into; choose another", ErrBadAccountName, acct)
	}
	if actor.KindOf(acct) != actor.None {
		return "", fmt.Errorf("%w: %q reads as a key; choose another", ErrBadAccountName, acct)
	}
	if vendor == "" {
		vendor = DefaultVendor
	}
	if err := CheckVendor(vendor); err != nil {
		return "", err
	}
	if slices.Contains(paths.ListAccounts(a.Home), acct) {
		return "", fmt.Errorf("%w: account '%s' already exists", ErrAccountExists, acct)
	}
	// Its row first: the folder is named by the row's key.
	d, err := store.SharedIfThere(paths.DBFile(a.Home))
	if err != nil {
		return "", err
	}
	if d == nil {
		return "", fmt.Errorf("no database in %s: start the daemon once", a.Home)
	}
	owner, err := actor.Owner(d)
	if err != nil {
		return "", fmt.Errorf("account %s: %w", acct, err)
	}
	res, err := d.Exec("INSERT INTO accounts(name, owner, created_at) VALUES(?, ?, ?) ON CONFLICT(name) DO NOTHING", acct, owner, time.Now().Unix())
	if err != nil {
		return "", fmt.Errorf("account %s: %w", acct, err)
	}
	madeRow, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("account %s: %w", acct, err)
	}
	dir := paths.AccountDir(a.Home, acct)
	dropRow := func() {
		if madeRow > 0 {
			d.Exec("DELETE FROM accounts WHERE name = ?", acct)
		}
	}
	if err := os.MkdirAll(paths.AccountsDir(a.Home), 0o755); err != nil {
		dropRow()
		return "", err
	}
	// Made alone, so of two adds of one name at once the second finds it there, and leaves the row the first one uses.
	if err := os.Mkdir(dir, 0o755); os.IsExist(err) {
		return "", fmt.Errorf("%w: account '%s' already exists", ErrAccountExists, acct)
	} else if err != nil {
		dropRow()
		return "", err
	}
	cdir := filepath.Join(dir, ConfigDirName(vendor))
	fail := func(err error) (string, error) {
		os.RemoveAll(dir)
		dropRow()
		return "", err
	}
	if err := os.Mkdir(cdir, 0o755); err != nil {
		return fail(err)
	}
	if vendor != DefaultVendor {
		if err := os.WriteFile(filepath.Join(dir, "vendor"), []byte(vendor+"\n"), 0o644); err != nil {
			return fail(err)
		}
	}
	shared := filepath.Join(a.Home, "shared", ConfigDirName(vendor))
	for _, item := range sharedItems(vendor) {
		target := filepath.Join(shared, item)
		if _, err := os.Stat(target); err != nil {
			continue // not made yet: the tick's relink adds it once it is
		}
		if err := os.Symlink(target, filepath.Join(cdir, item)); err != nil {
			return fail(fmt.Errorf("%s failed (%w); nothing was kept", item, err))
		}
	}
	return cdir, nil
}

// RemovePreview is what Remove would do to the account, line by line, and whether it would go ahead. It changes nothing.
func (a Accounts) RemovePreview(q *sql.DB, acct string) (would bool, lines []string, err error) {
	ok, why, err := a.RemovePlan(q, acct)
	if err != nil {
		return false, nil, err
	}
	if !ok {
		return false, []string{"would refuse: " + why}, nil
	}
	lines = []string{fmt.Sprintf("would remove account '%s':", acct)}
	if argv, _ := a.LogoutArgv(acct); len(argv) > 0 {
		lines = append(lines, "  log it out with `"+strings.Join(argv, " ")+"`")
	} else {
		lines = append(lines, "  log it out: not logged in, nothing to do")
	}
	lines = append(lines, fmt.Sprintf("  move %s to %s/%s-<time> (kept, not deleted)", paths.AccountDir(a.Home, acct), filepath.Join(paths.AccountsDir(a.Home), ".retired"), acct))
	rows, err := q.Query("SELECT agent FROM agent_state WHERE account = ?", acct)
	if err != nil {
		return false, nil, err
	}
	var on []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return false, nil, err
		}
		on = append(on, n)
	}
	rows.Close()
	if len(on) > 0 {
		lines = append(lines, fmt.Sprintf("  forget it on agent(s) %s (they start fresh on whatever account has room)", strings.Join(on, ", ")))
	} else {
		lines = append(lines, "  forget it on no agent (none is recorded on it)")
	}
	return true, append(lines, "  clear its usage polls and login check"), nil
}

// RemoveRefused is a removal that did not go ahead, and why; nothing was changed unless the message says a logout ran.
type RemoveRefused struct{ Why string }

func (e *RemoveRefused) Error() string { return e.Why }

// Remove logs the account out and takes it off the roster (accounts.remove_account), the confirmation having been asked already. run runs
// the vendor's logout and answers its exit code (-1 when it could not run). Refused while an agent is on the account and when the logout
// fails. The directory is moved aside, never deleted: the vendor removes its own credential, and moving the
// directory of a logged-in account would orphan it. Answers the line to say.
func (a Accounts) Remove(q *sql.DB, acct string, now time.Time, run func(argv []string, env map[string]string) int) (string, error) {
	ok, why, err := a.RemovePlan(q, acct)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &RemoveRefused{why}
	}
	if CheckVendor(AccountVendor(a.Home, acct)) == nil { // a live run's account the real home lacks: its logout would run elsewhere
		if _, err := AccountConfigDir(a.Home, acct); err != nil {
			return "", &RemoveRefused{err.Error()}
		}
	}
	marker := filepath.Join(paths.AccountDir(a.Home, acct), ".removing") // nothing starts on an account about to go
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		return "", err
	}
	defer os.Remove(marker)
	if ok, why, err = a.RemovePlan(q, acct); err != nil {
		return "", err
	} else if !ok {
		return "", &RemoveRefused{why}
	}
	if argv, env := a.LogoutArgv(acct); len(argv) > 0 {
		if code := run(argv, env); code != 0 || a.LoggedIn(acct) {
			return "", &RemoveRefused{fmt.Sprintf("logout failed - '%s' is still logged in, so it stays. Run `%s` with %s=%s and try again.",
				acct, strings.Join(argv, " "), ConfigDirEnv(AccountVendor(a.Home, acct)), mustConfigDir(a.Home, acct))}
		}
	}
	os.Remove(marker)
	dir := paths.AccountDir(a.Home, acct) // by its row's key, so before the row goes
	// The rows first, in one transaction: a removal stopped halfway leaves no row a new account of the name would take over.
	if err := inTx(q, func(tx db.Q) error {
		// The name can be added again, and a poll remembered against the old account would hold the new one in its cooldown.
		for _, kind := range []string{"usage-poll", "usage-refresh", "login-check"} {
			if err := db.ClearCursor(tx, kind+":"+acct); err != nil {
				return err
			}
		}
		// A closed agent's resume id lives under the directory moved aside: it starts fresh on whatever account has room.
		if _, err := tx.Exec("UPDATE agent_state SET account = NULL, account_source = NULL, resume_session = NULL WHERE account = ?", acct); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM accounts WHERE name = ?", acct) // a new account of the name starts at the defaults
		return err
	}); err != nil {
		return "", err
	}
	dest, err := a.retire(dir, acct, now)
	if err != nil {
		return "", fmt.Errorf("account '%s' is off the roster, but its folder %s was not moved aside: %w", acct, dir, err)
	}
	return fmt.Sprintf("removed account '%s' (logged out; directory kept at %s)", acct, dest), nil
}

func mustConfigDir(home, acct string) string {
	d, _ := AccountConfigDir(home, acct)
	return d
}
