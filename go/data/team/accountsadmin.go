package team

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
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
// with the reason); a vendor with no driver is *UnknownVendor; a name in use is ErrAccountExists. extra are links its config dir gets
// too (AdoptLink). Returns the config directory made.
func (a Accounts) Add(acct, vendor string, extra ...Link) (string, error) {
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
	// Its row first: it records the folder, which AccountDir then reads.
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
	folder, err := newAccountFolder(d, a.Home)
	if err != nil {
		return "", fmt.Errorf("account %s: %w", acct, err)
	}
	res, err := d.Exec("INSERT INTO accounts(name, owner, created_at, folder) VALUES(?, ?, ?, ?) ON CONFLICT(name) DO NOTHING", acct, owner,
		time.Now().Unix(), folder)
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
	for _, l := range extra {
		if err := os.Symlink(l.Target, filepath.Join(cdir, l.Name)); err != nil {
			return fail(fmt.Errorf("linking %s failed (%w); nothing was kept", l.Name, err))
		}
	}
	return cdir, nil
}

// newAccountFolder is a free random name for a new account's folder (FolderName): hex, so it says nothing of the order accounts were
// added in, and never one that reads as a key (a key no row has is its own folder).
func newAccountFolder(q actor.Q, home string) (string, error) {
	b := make([]byte, 3)
	for range 100 {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		f := hex.EncodeToString(b)
		var taken int
		if err := q.QueryRow("SELECT COUNT(*) FROM accounts WHERE folder = ?", f).Scan(&taken); err != nil {
			return "", err
		}
		_, err := os.Lstat(filepath.Join(paths.AccountsDir(home), f))
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if taken == 0 && actor.KindOf(f) == actor.None && err != nil {
			return f, nil
		}
	}
	return "", errors.New("no free folder name found")
}

// FolderName is a name shaped as newAccountFolder makes one: a lent machine's account folder named so is no name for an account.
var FolderName = regexp.MustCompile(`^[0-9a-f]{6}$`)

// byEmail makes ByEmail's find and add one step: two machines naming one new email at once make one account.
var byEmail sync.Mutex

// OfEmail is the claude account logged in as email, or recorded with it, "" for none: an email is one account on every machine
// (docs/several-machines.md section 1 item 3).
func (a Accounts) OfEmail(email string) string {
	if email == "" {
		return ""
	}
	for _, acct := range paths.ListAccounts(a.Home) {
		if AccountVendor(a.Home, acct) == DefaultVendor && strings.EqualFold(a.Email(acct), email) {
			return acct
		}
	}
	return ""
}

// ByEmail is OfEmail, else an account added for email (as Add: logged in nowhere here) with email recorded. It is named name, or, when
// name is taken or not a valid account name, after the email; -2, -3 ... when that is taken too.
func (a Accounts) ByEmail(email, name string) (string, error) {
	if email == "" {
		return "", errors.New("no email to find an account by")
	}
	byEmail.Lock()
	defer byEmail.Unlock()
	if acct := a.OfEmail(email); acct != "" {
		return acct, nil
	}
	local, _, _ := strings.Cut(strings.ToLower(email), "@")
	local = strings.Trim(nonName.ReplaceAllString(local, "-"), "-")
	if FolderName.MatchString(name) {
		name = ""
	}
next:
	for _, base := range []string{name, local} {
		for n := 1; n <= 20 && base != ""; n++ {
			acct := base
			if n > 1 {
				acct = fmt.Sprintf("%s-%d", base, n)
			}
			_, err := a.Add(acct, DefaultVendor)
			switch {
			case err == nil:
				if err := a.recordEmail(acct, email); err != nil { // with no email it is nobody's: not kept, or the next look makes another
					return "", errors.Join(err, a.unadd(acct))
				}
				return acct, nil
			case errors.Is(err, ErrBadAccountName):
				continue next
			case !errors.Is(err, ErrAccountExists):
				return "", err
			}
		}
	}
	return "", fmt.Errorf("no free account name for %s", email)
}

// unadd takes away an account Add just made, which nothing uses yet.
func (a Accounts) unadd(acct string) error {
	d, err := store.SharedIfThere(paths.DBFile(a.Home))
	if err != nil || d == nil {
		return err
	}
	dir := paths.AccountDir(a.Home, acct)
	if _, err := d.Exec("DELETE FROM accounts WHERE name = ?", acct); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// nonName is what an account named after an email leaves out.
var nonName = regexp.MustCompile(`[^a-z0-9_-]+`)

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
	lines = append(lines, fmt.Sprintf("  delete %s", paths.AccountDir(a.Home, acct)))
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
// fails. The vendor removes its own credential first; then the directory is deleted (the conversations stay in the database). Answers
// the line to say.
func (a Accounts) Remove(q *sql.DB, acct string, run func(argv []string, env map[string]string) int) (string, error) {
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
	dir := paths.AccountDir(a.Home, acct) // by its row, so before the row goes
	// The rows first, in one transaction: a removal stopped halfway leaves no row a new account of the name would take over.
	if err := inTx(q, func(tx db.Q) error {
		// The name can be added again, and a poll remembered against the old account would hold the new one in its cooldown.
		for _, kind := range []string{"usage-poll", "usage-refresh", "login-check"} {
			if err := db.ClearCursor(tx, kind+":"+acct); err != nil {
				return err
			}
		}
		// A closed agent's resume id lives under the directory deleted: it starts fresh on whatever account has room.
		if _, err := tx.Exec("UPDATE agent_state SET account = NULL, account_source = NULL, resume_session = NULL WHERE account = ?", acct); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM accounts WHERE name = ?", acct) // a new account of the name starts at the defaults
		return err
	}); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("account '%s' is off the roster, but its folder %s was not deleted: %w", acct, dir, err)
	}
	return fmt.Sprintf("removed account '%s' (logged out, folder deleted)", acct), nil
}

func mustConfigDir(home, acct string) string {
	d, _ := AccountConfigDir(home, acct)
	return d
}
