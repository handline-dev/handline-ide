<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# repos methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `repos.list` | member |  | none | `{repos}` |  |  | `repos`, snapshot |
| `repos.recent` | member |  | none | `{folders}` |  |  | `recent_repos`, snapshot |
| `worktrees.list` | member |  | `repo?`, `owner?` | `{worktrees}` |  |  | `worktrees`, snapshot |
| `worktree.diff` | member |  | `path`, `file?`, `against?` | `{path, file, diff, truncated, against, base_ref}` | `not_found` |  | `worktree-diff`, snapshot |
| `files.list` | member |  | `agent`, `query?`, `limit?` | `{files, more}` | `bad_request`, `not_found` |  | `files`, snapshot |
| `file.read` | founder |  | `agent`, `path` | `{text, size, truncated, mtime, etag}` | `bad_request`, `not_found`, `refused` |  | `file`, snapshot |
| `file.write` | founder | yes | `agent`, `path`, `text`, `etag`, `request_id` | `{size, mtime, etag}` | `bad_request`, `not_found`, `refused` | denied, stale, too_big | decided: save file |
| `worktree.set_status` | member | yes | `path`, `status`, `request_id` | `Accepted` | `not_found`, `bad_request` |  | decided: set status |
| `doc.open` | founder |  | `agent`, `path` | `{doc, version, text}` | `bad_request`, `not_found`, `refused` | too_big | not cached |
| `doc.push` | founder | yes | `agent`, `path`, `doc`, `version`, `updates`, `request_id` | `{version}` | `bad_request`, `not_found`, `refused` | behind, reopen, too_big | uncached: edit |
| `doc.pull` | founder |  | `agent`, `path`, `doc`, `version` | `{version, updates}` | `bad_request`, `not_found`, `refused` | reopen | not cached |
| `doc.close` | founder |  | `agent`, `path` | `object` | `bad_request` |  | not cached |

### `repos.list`

Repos.

### `repos.recent`

Git folders under the person's home not in repos.list, for the first-run repo pick: at most 12, newest first. Found by folder stats alone, up to three levels below home, within about 300 ms; no symlink, other volume, dot folder, app bundle, media library, ~/Documents, ~/Desktop, ~/Downloads, ~/Library, ~/Pictures, ~/Music, ~/Movies or ~/Applications is entered.

### `worktrees.list`

Worktrees with status, changed files, what each holds beyond its base and its pull request. The worktrees agents work in, each with what it holds that main does not and its size. `repo` narrows them, by the repo's name or path.

### `worktree.diff`

Read-only git diff run by the daemon. Without `file`: the changes to tracked files against HEAD (`against` head, the default), then a new-file diff of each untracked file that .gitignore does not ignore (at most 200; one too big for what is left of the size cap is listed as `(new file, N bytes: not shown)`, not diffed). With `file`: that file alone, untracked or not.

### `files.list`

The files of an agent's worktree, for the chat's @ menu. The files are what git tracks plus the untracked files it does not ignore (`git ls-files --cached --others --exclude-standard`), the regular files among them that are there now. An agent with no worktree, or whose worktree is gone, is not_found. It reads only names and sizes, never a file's content.

### `file.read`

The founder reads a text file in an agent's worktree.

### `file.write`

The founder replaces or creates a .md or .json file in an agent's worktree, if it is still what the etag says.

### `worktree.set_status`

Set the status label of a worktree. The status is a free string (Worktree.status), 1 to 40 characters; the label changes nothing on disk, and worktrees.list shows it in place of the daemon's own word (dirty or clean). A path that is not a worktree an agent works in is not_found, what worktree, as for worktree.diff.

### `doc.open`

The founder opens a .md file of an agent's worktree as a shared document: the daemon holds the text, orders every edit, and writes it to the file. The connection gets doc_changed and doc_closed notices for the doc until doc.close or until it closes. Opening a doc the connection has open again is answered the same and sends nothing twice. The text has \n line breaks: a file written with \r\n is held with \n and written back with \r\n. not_found: the file is not there. too_big: the file is over 1 MiB. bad_request (field path): not a file, not UTF-8 text without NUL, or a \r that is not before \n. Path rules and the denied places are file.write's.

### `doc.push`

An editor sends its updates to a shared document, based on the version it has. The updates are applied in order, each on the document the one before it left; every connection that has the doc open (the pusher too) gets one doc_changed with them, each carrying by `human`. The daemon writes the file about 300 ms after the last push and at the last close. Before each write it reads the file: a change made there that it did not write is first taken in as one update (client `disk`, sent with doc_changed), so the write keeps it; an outside change is never written over. The two are merged line by line: where an editor typed inside a line the outside change also changed, the outside change's line is kept. A write that fails is tried again; a doc whose last editor closed it stays loaded until its write is made. A file found deleted or unreadable then is let go (doc_closed), not written again. A retry with the same request_id is judged again, not replayed: if the first was taken, it answers behind, and doc.pull shows the updates (collab knows its own by client). behind: version is not the current one (data.version is): nothing is taken; pull, rebase, push again. reopen: doc is not the loaded copy (the daemon loaded the file again): open it again. too_big: the document would be over 1 MiB. bad_request (field updates): a ChangeSet whose lengths do not add up to the document it applies to, a line with \r or NUL, or one that cuts a character in two; nothing is taken. bad_request (field path or doc) as doc.open's path, or a missing doc. The doc must be open on this connection: not_found otherwise.

### `doc.pull`

The updates a shared document took after a version, to catch up. updates are those after `version`, in order, and `version` the doc's current one: none when the caller is current. reopen: doc is not the loaded copy, or the version is older than the updates the daemon keeps (the last 1000): open it again. bad_request (field version): above the current version; (field path or doc) as doc.push. The doc must be open on this connection: not_found otherwise.

### `doc.close`

Stop receiving a shared document on this connection. Not an error when the doc is not open here; bad_request (field path) as doc.open's path. When no connection has it open any more, the daemon writes what it has not written (as doc.push says) and lets it go. Closing the connection closes its docs.

