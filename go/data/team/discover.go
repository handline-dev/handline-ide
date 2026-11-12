package team

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
)

// Found is a login already on this Mac, outside Handline (accounts.discover): the vendor, its folder as ~/<name>, whether the vendor's
// own check says it is logged in (nil: it cannot be told) and whether Adopt takes it.
type Found struct {
	Vendor    string
	Source    string
	LoggedIn  *bool
	Adoptable bool
}

// discoverable is the vendors whose own folder in the person's home Discover looks in, and that folder; claude first.
var discoverable = []struct{ vendor, dir string }{{"claude", ".claude"}, {"codex", ".codex"}}

// Discover is each discoverable folder that exists, told from the vendor's credential check only, never a credential's content. A
// vendor with no driver here yet is found with its login unknown and not adoptable.
func Discover() []Found {
	out := []Found{}
	for _, d := range discoverable {
		if st, err := os.Stat(filepath.Join(agent.UserHome(), d.dir)); err != nil || !st.IsDir() {
			continue
		}
		f := Found{Vendor: d.vendor, Source: "~/" + d.dir}
		if a, ok := agent.Lookup(d.vendor); ok {
			if in, known := a.CredentialPresent(filepath.Join(agent.UserHome(), d.dir)); known {
				f.LoggedIn = &in
			}
			_, f.Adoptable = a.(agent.Sharable)
		}
		out = append(out, f)
	}
	return out
}

// ErrNotAdoptable, ErrNoSource and ErrSourceLoggedOut are AdoptLink's refusals: a vendor whose login cannot be shared, a source
// Discover did not answer for it, and one it found logged out.
var (
	ErrNotAdoptable    = errors.New("cannot be shared with an account: add the account and log it in")
	ErrNoSource        = errors.New("no such login on this Mac")
	ErrSourceLoggedOut = errors.New("that login is not logged in")
)

// Link is a file an account's config dir links to outside Handline: Name in the config dir, pointing at Target.
type Link struct{ Name, Target string }

// AdoptLink is the one link an account needs to use the vendor login Discover found at source (account.adopt): only its credential
// file, so the account's hooks, instructions and sessions stay out of the person's folder.
func AdoptLink(vendor, source string) (Link, error) {
	drv, registered := agent.Lookup(vendor)
	share, sharable := drv.(agent.Sharable)
	if !slices.ContainsFunc(discoverable, func(d struct{ vendor, dir string }) bool { return d.vendor == vendor }) || registered && !sharable {
		return Link{}, fmt.Errorf("a %q login %w", vendor, ErrNotAdoptable)
	}
	found := Discover()
	i := slices.IndexFunc(found, func(f Found) bool { return f.Vendor == vendor && f.Source == source })
	if i < 0 {
		return Link{}, fmt.Errorf("%w: %s", ErrNoSource, source)
	}
	if in := found[i].LoggedIn; in == nil || !*in || !sharable { // a vendor with no driver yet cannot tell its login
		return Link{}, fmt.Errorf("%w: %s", ErrSourceLoggedOut, source)
	}
	file := share.CredentialFile()
	return Link{Name: file, Target: filepath.Join(agent.UserHome(), strings.TrimPrefix(source, "~/"), file)}, nil
}
