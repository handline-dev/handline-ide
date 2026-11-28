package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// where is the page's title and address, as open says them.
func (p *Page) where() (string, error) {
	var w struct{ Title, URL string }
	if err := p.eval(`({Title: document.title, URL: location.href})`, &w); err != nil {
		return "", err
	}
	return strings.TrimSpace(w.Title + "  " + w.URL), nil
}

// Go moves by steps in the page's history (-1 is back) and waits for the page there to load.
func (p *Page) Go(steps int) (string, error) {
	raw, err := p.c.call("Page.getNavigationHistory", nil)
	if err != nil {
		return "", err
	}
	var h struct {
		Current int `json:"currentIndex"`
		Entries []struct {
			ID  int    `json:"id"`
			URL string `json:"url"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return "", err
	}
	to := h.Current + steps
	if to < 0 || to >= len(h.Entries) {
		word := "forward"
		if steps < 0 {
			word = "back"
		}
		return "", errors.New("there is no page to go " + word + " to")
	}
	if _, err := p.c.call("Page.navigateToHistoryEntry", map[string]any{"entryId": h.Entries[to].ID}); err != nil {
		return "", err
	}
	for end := time.Now().Add(loadWithin); ; time.Sleep(50 * time.Millisecond) {
		// While the page changes process the page is not there to ask: an error is asked again.
		raw, err := p.c.call("Page.getNavigationHistory", nil)
		var now struct {
			Current int `json:"currentIndex"`
		}
		var doc struct{ Ready, URL string } // the history moves before the page there commits: the document says when it has
		if err == nil && json.Unmarshal(raw, &now) == nil && now.Current == to &&
			p.eval(`({Ready: document.readyState, URL: location.href})`, &doc) == nil && doc.Ready == "complete" && doc.URL == h.Entries[to].URL {
			return p.where()
		}
		if time.Now().After(end) {
			if err == nil {
				err = errors.New("it is still loading")
			}
			return "", fmt.Errorf("the page did not load within %s: %w", loadWithin, err)
		}
	}
}

// Reload loads the page again and waits for it.
func (p *Page) Reload() (string, error) {
	loads := 0
	prev := p.c.on
	p.c.on = func(m message) {
		if m.Method == "Page.loadEventFired" {
			loads++
		}
		prev(m)
	}
	defer func() { p.c.on = prev }()
	if _, err := p.c.call("Page.reload", nil); err != nil {
		return "", err
	}
	deadline := time.Now().Add(loadWithin)
	for loads == 0 {
		if _, err := p.c.read(deadline); err != nil {
			return "", fmt.Errorf("the page did not load again within %s: %w", loadWithin, err)
		}
	}
	return p.where()
}

// Tab is one of the browser's pages: its number in hl browser tab, and whether it is the one the agent works in.
type Tab struct {
	N          int
	ID         string
	Title, URL string
	Current    bool
}

func (b *Browser) pages() ([]target, error) {
	resp, err := b.endpoint("/json/list", http.MethodGet)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list []target
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(t target) bool { return t.Type != "page" || strings.HasPrefix(t.URL, "devtools://") }), nil
}

// Tabs lists the pages, numbered in the order they were first seen, so a number stays with its page.
func (b *Browser) TabList() ([]Tab, error) {
	list, err := b.pages()
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	err = b.update(func(b *Browser) {
		byID := map[string]target{}
		for _, t := range list {
			byID[t.ID] = t
		}
		b.Tabs = slices.DeleteFunc(b.Tabs, func(id string) bool { _, ok := byID[id]; return !ok })
		for i := len(list) - 1; i >= 0; i-- { // the list is newest first
			if !slices.Contains(b.Tabs, list[i].ID) {
				b.Tabs = append(b.Tabs, list[i].ID)
			}
		}
		if _, ok := byID[b.PageID]; !ok && len(b.Tabs) > 0 { // the tab worked in closed: the first one left is worked in
			b.PageID = b.Tabs[0]
		}
		for i, id := range b.Tabs {
			t := byID[id]
			tabs = append(tabs, Tab{N: i + 1, ID: id, Title: t.Title, URL: t.URL, Current: id == b.PageID})
		}
	})
	return tabs, err
}

func (b *Browser) tab(n int) (Tab, error) {
	tabs, err := b.TabList()
	if err != nil {
		return Tab{}, err
	}
	if n < 1 || n > len(tabs) {
		return Tab{}, fmt.Errorf("there is no tab %d: hl browser tab lists them", n)
	}
	return tabs[n-1], nil
}

func (b *Browser) workIn(id string) error {
	return b.update(func(b *Browser) { b.PageID = id })
}

// TabTo makes tab n the page the agent works in.
func (b *Browser) TabTo(n int) error {
	t, err := b.tab(n)
	if err != nil {
		return err
	}
	if resp, err := b.endpoint("/json/activate/"+t.ID, http.MethodGet); err == nil {
		resp.Body.Close()
	}
	return b.workIn(t.ID)
}

// TabNew opens a new page and works in it.
func (b *Browser) TabNew() error {
	resp, err := b.endpoint("/json/new?"+url.QueryEscape("about:blank"), http.MethodPut)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var t target
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return err
	}
	if _, err := b.TabList(); err != nil {
		return err
	}
	return b.workIn(t.ID)
}

// TabClose closes tab n; closing the one worked in moves the work to the first tab left.
func (b *Browser) TabClose(n int) error {
	t, err := b.tab(n)
	if err != nil {
		return err
	}
	resp, err := b.endpoint("/json/close/"+t.ID, http.MethodGet)
	if err != nil {
		return err
	}
	resp.Body.Close()
	_, err = b.TabList()
	return err
}

// Download clicks target and waits within for the file it starts to land in dir, and says its path.
func (p *Page) Download(at Box, dir string, within time.Duration) (string, error) {
	bc, err := p.b.dialBrowser()
	if err != nil {
		return "", err
	}
	defer bc.close()
	if _, err := bc.call("Browser.setDownloadBehavior", map[string]any{"behavior": "allowAndName", "downloadPath": dir,
		"eventsEnabled": true}); err != nil {
		return "", err
	}
	defer bc.call("Browser.setDownloadBehavior", map[string]any{"behavior": "default"})
	var name, guid, state string
	bc.on = func(m message) {
		var e struct{ GUID, SuggestedFilename, State string }
		_ = json.Unmarshal(m.Params, &e)
		switch {
		case m.Method == "Browser.downloadWillBegin" && guid == "":
			guid, name = e.GUID, e.SuggestedFilename
		case m.Method == "Browser.downloadProgress" && e.GUID == guid && e.State != "inProgress":
			state = e.State
		}
	}
	if err := p.ClickAt(at); err != nil {
		return "", err
	}
	deadline := time.Now().Add(within)
	for state == "" {
		if _, err := bc.read(deadline); err != nil {
			if guid == "" {
				return "", fmt.Errorf("the click started no download within %s", within)
			}
			return "", fmt.Errorf("%s did not finish downloading within %s", name, within)
		}
	}
	if state != "completed" {
		return "", fmt.Errorf("the download of %s was %s", name, state)
	}
	return keep(filepath.Join(dir, guid), dir, name)
}

// keep names the downloaded file as the site named it, beside any file of that name already there.
func keep(file, dir, name string) (string, error) {
	name = filepath.Base(name)
	if name == "." || name == "/" || name == "" {
		name = "download"
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	to := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Lstat(to); errors.Is(err, os.ErrNotExist) {
			break
		}
		to = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
	}
	return to, os.Rename(file, to)
}
