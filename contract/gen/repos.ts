// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, AgentName, Repo, RequestId, Role, Timestamp, Topic, WriteKind } from './core.ts';

export type ChangedFile = {
  path: string;
  /** M, A, D, R or ?. */
  status: string;
  added: number;
  deleted: number;
};

export type WorktreePR = {
  number: number;
  url: string;
  /** merged: it was merged. draft: it is a draft. checks_failing: a check failed (`failing` counts them). changes_requested: a reviewer asked for changes. waiting_review: a review is required and none approved. ready: nothing holds it. */
  verdict: "ready" | "checks_failing" | "waiting_review" | "changes_requested" | "draft" | "merged";
  /** Present only when verdict is checks_failing: how many checks failed. */
  failing?: number;
};

export type Worktree = {
  path: string;
  repo: string;
  branch: string;
  owner: AgentName | null;
  parent: string | null;
  children: string[];
  status: string;
  changed: ChangedFile[];
  /** The branch it merges into: its repo's default branch. */
  base: string;
  /** Its changed and untracked files, as git status counts them. */
  files_changed: number;
  /** Its commits not on base. */
  commits_ahead: number;
  /** The bytes it takes on disk, as housekeeping last measured them (at most hourly, off the daemon's tick); null until measured. */
  size_bytes: number | null;
  /** The pull request of its branch, read with the daemon's own gh login and kept about a minute; null when the branch has none open or merged, it is base itself, or gh could not answer (never a guess). */
  pr: WorktreePR | null;
};

/** CodeMirror's ChangeSet.toJSON(): in document order, an integer is that many characters kept, and an array [n, ...lines] replaces n characters with the lines joined by \n ([n] deletes them). Lengths count UTF-16 code units, as CodeMirror does. The kept and replaced lengths add up to the document's length. */
export type ChangeSet = (number | ((number | string)[]))[];

export type DocUpdate = {
  /** The pushing editor's client id, which collab matches its own updates by; `disk` for a change made to the file outside the doc. */
  client: string;
  changes: ChangeSet;
  /** Who made it: `human` for a push; for a disk change, the agent whose Edit, Write or MultiEdit named the file in the 10 s before it was seen; absent when the daemon cannot tell. */
  by?: string;
};

export type RecentFolder = {
  path: string;
  /** The folder's own name. */
  name: string;
  /** The later mtime of its .git/HEAD and .git/index. */
  touched_at: Timestamp;
};

export type RepoFacts = {
  /** What runs its tests, by the files at its top: a package.json test script (pnpm, yarn or bun test by lockfile, bun.lock or bun.lockb for bun, else npm test), go.mod (go test ./...), a pytest or Python project file (pytest), a Makefile test target (make test), in that order; null for none. */
  test_command: string | null;
  /** The two newest open issues of its GitHub repo, by gh within 3 s; null when gh cannot tell. */
  issues: {
    number: number;
    title: string;
  }[] | null;
  /** Lines of tracked text files holding the word TODO or FIXME, at most 999. */
  todos: number;
  /** Files git status lists, untracked ones included. */
  uncommitted: number;
};

export interface ReposMethods {
  "repos.list": { params: { [key: string]: unknown }; result: {
    repos: Repo[];
  } };
  "repos.recent": { params: { [key: string]: unknown }; result: {
    folders: RecentFolder[];
  } };
  "repo.facts": { params: {
  path: string;
}; result: RepoFacts };
  "worktrees.list": { params: {
  repo?: string;
  owner?: AgentName;
}; result: {
    worktrees: Worktree[];
  } };
  "worktree.diff": { params: {
  path: string;
  file?: string;
  /** head (absent): against HEAD. base: against the merge-base with the base branch, so the branch's commits show too; no base ref answers as head. */
  against?: "head" | "base";
}; result: {
    path: string;
    file: string | null;
    /** Unified diff. */
    diff: string;
    truncated: boolean;
    /** What the diff was taken against: base falls back to head when the repo has no base ref. */
    against: "head" | "base";
    /** The ref `base` measured against, e.g. origin/main; null when against is head. */
    base_ref: string | null;
  } };
  "files.list": { params: {
  agent: AgentName;
  /** Matched case-insensitively as a subsequence of the path: its letters appear in the path in order, not necessarily side by side. Absent, null or empty lists every file. Anything else that is not a string is bad_request (field query). */
  query?: string;
  /** The most files returned; absent or null is 50. Outside 1..200, or not an integer: bad_request (field limit). */
  limit?: number;
}; result: {
    /** With no query, every file in byte order of path. With one, the matches best first: a match inside the file's name (the part after the last `/`) comes first, then one elsewhere in the path, then a subsequence that is not side by side; within each, the shorter path first, then byte order. */
    files: {
      /** Relative to the worktree, with `/` between parts. */
      path: string;
      /** The file's size in bytes. */
      size: number;
    }[];
    /** More files matched than limit let through. */
    more: boolean;
  } };
  "file.read": { params: {
  agent: AgentName;
  /** Relative to the agent's worktree, with `/` between parts. No .., no empty or . part, not absolute, no backslash, and no part of it (nor the worktree path under it) may be a symlink: bad_request (field path) otherwise. */
  path: string;
}; result: {
    /** UTF-8 text; the first 1 MiB when the file is longer (truncated). */
    text: string;
    /** The file's size in bytes. */
    size: number;
    /** The file is over 1 MiB and text is its first 1 MiB. Do not write it back. */
    truncated: boolean;
    mtime: Timestamp;
    /** SHA-256 of the whole file, hex: file.write takes it back. */
    etag: string;
  } };
  "file.write": { params: {
  agent: AgentName;
  /** Relative to the agent's worktree, with `/` between parts. No .., no empty or . part, not absolute, no backslash, and no part of it (nor the worktree path under it) may be a symlink: bad_request (field path) otherwise. */
  path: string;
  /** The whole new content: UTF-8 text without NUL, at most 1 MiB; for a .json file, valid JSON. */
  text: string;
  /** The etag file.read gave; empty to create a file that must not exist (its folder must). */
  etag: string;
  request_id: RequestId;
}; result: {
    size: number;
    mtime: Timestamp;
    /** The new file's etag. */
    etag: string;
  } };
  "worktree.set_status": { params: {
  path: string;
  status: string;
  request_id: RequestId;
}; result: Accepted };
  "doc.open": { params: {
  agent: AgentName;
  /** As file.write's path, and a .md file: bad_request (field path) otherwise. */
  path: string;
}; result: {
    /** The id of this loaded copy. The daemon loads the file again (a new id, versions from 0) after the last connection closed it. */
    doc: string;
    /** How many updates the doc has taken since it was loaded. */
    version: number;
    /** The document at that version. */
    text: string;
  } };
  "doc.push": { params: {
  agent: AgentName;
  /** As file.write's path, and a .md file: bad_request (field path) otherwise. */
  path: string;
  /** The doc id doc.open gave. */
  doc: string;
  /** The version the updates were made on: must be the doc's current one. */
  version: number;
  updates: {
    client: string;
    changes: ChangeSet;
  }[];
  request_id: RequestId;
}; result: {
    /** The doc's version after the updates. */
    version: number;
  } };
  "doc.pull": { params: {
  agent: AgentName;
  /** As file.write's path, and a .md file: bad_request (field path) otherwise. */
  path: string;
  /** The doc id doc.open gave. */
  doc: string;
  /** The version the caller has. */
  version: number;
}; result: {
    version: number;
    updates: DocUpdate[];
  } };
  "doc.close": { params: {
  agent: AgentName;
  /** As file.write's path, and a .md file: bad_request (field path) otherwise. */
  path: string;
}; result: { [key: string]: unknown } };
}

export const REPOS_METHODS: { [K in keyof ReposMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "repos.list": { role: "member", callers: null, write: false, since: 1 },
  "repos.recent": { role: "member", callers: null, write: false, since: 1 },
  "repo.facts": { role: "member", callers: null, write: false, since: 1 },
  "worktrees.list": { role: "member", callers: null, write: false, since: 1 },
  "worktree.diff": { role: "member", callers: null, write: false, since: 1 },
  "files.list": { role: "member", callers: null, write: false, since: 1 },
  "file.read": { role: "founder", callers: null, write: false, since: 1 },
  "file.write": { role: "founder", callers: null, write: true, since: 1 },
  "worktree.set_status": { role: "member", callers: null, write: true, since: 1 },
  "doc.open": { role: "founder", callers: null, write: false, since: 1 },
  "doc.push": { role: "founder", callers: null, write: true, since: 1 },
  "doc.pull": { role: "founder", callers: null, write: false, since: 1 },
  "doc.close": { role: "founder", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const REPOS_APP_CACHE: { [K in "repos.list" | "repos.recent" | "repo.facts" | "worktrees.list" | "worktree.diff" | "files.list" | "file.read" | "doc.open" | "doc.pull" | "doc.close"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "repos.list": {"resource": "repos", "feed": null},
  "repos.recent": {"resource": "recent_repos", "feed": null},
  "repo.facts": {"resource": "repo_facts", "feed": null},
  "worktrees.list": {"resource": "worktrees", "feed": null},
  "worktree.diff": {"resource": "worktree-diff", "feed": null},
  "files.list": {"resource": "files", "feed": null},
  "file.read": {"resource": "file", "feed": null},
  "doc.open": false,
  "doc.pull": false,
  "doc.close": false,
};

/** x-app-write of each write (x-app in core.json). */
export const REPOS_APP_WRITES: { [K in "file.write" | "worktree.set_status" | "doc.push"]: { kind: WriteKind; label: string } | false } = {
  "file.write": {"kind": "decided", "label": "save file"},
  "worktree.set_status": {"kind": "decided", "label": "set status"},
  "doc.push": {"kind": "uncached", "label": "edit"},
};
