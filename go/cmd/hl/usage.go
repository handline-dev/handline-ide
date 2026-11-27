package main

// usages is each verb's form and one example, printed when its arguments are refused: "<verb> <sub>" first, then "<verb>".
var usages = map[string][2]string{
	"say":      {`hl say <target> "<text>" [--parent <msg>] [--file PATH] [--attach FILE] (chat, queued until the target takes it; on a task, write a comment)`, `hl say builder "the build machine is free again"`},
	"read":     {`hl read <msg> [--wait] [--timeout <span>]`, `hl read msg_1a2b3c4`},
	"inbox":    {`hl inbox [--all] [--wait] [--timeout <span>]`, `hl inbox --all`},
	"withdraw": {`hl withdraw <msg> [--reason "<why>"]`, `hl withdraw msg_1a2b3c4 --reason "sent to the wrong agent"`},
	"progress": {`hl progress [--done N --of M] "<done: ... left: ...>"`, `hl progress --done 3 --of 5 "wiring the gate"`},
	"load":     {`hl load [--agent <name>]`, `hl load`},
	"desk":     {`hl desk list [<repo>]   |   hl desk pin <desk> <agent|founder>   |   hl desk unpin <desk>`, `hl desk pin lead-3f9a2c founder`},
	"lesson":   {`hl lesson add "<what to do differently>" --task T<n> [--about founder --said <comment|msg>]   |   hl lesson merge <id> <id>... "<one lesson for them>" [--about founder --said <comment|msg>]   |   hl lesson list [<agent>|founder]`, `hl lesson add "show the screen in both themes" --task T12`}, // local-ref-ok: the usage example
	"prefs":    {`hl prefs [agent]`, `hl prefs ui`},
	"team":     {`hl team [--agent <name>]`, `hl team`},
	"browser": {`hl browser open <url> [--show]   |   read [--tree]   |   click <ref|selector>   |   type <ref|selector> "<text>" [--enter]   |   wait (<ref|selector> | --text "<text>") [--timeout <span>]   |   eval "<script>"   |   shot <file.png> [--full]   |   close`,
		`hl browser click e12`},
	"use":        {`hl use <run|port>... -- <command...>   |   hl use <run|port> --hold   |   hl use --release [<run|port>]   |   hl use --extend [<run|port>]   |   hl use add|remove|list`, `hl use xcode port -- npm run e2e`},
	"use add":    {`hl use add <name> --cmd <command> [--cmd <command>]... [--at-once N] [--mac | --workspace <w>] --why "<why>" (a command: a program, a program and its first words, a script path, or @build-test, @simulator, @browser)`, `hl use add xcode --cmd xcodebuild --mac --why "one build at a time keeps the fans down"`},
	"use remove": {`hl use remove <name> [--mac | --workspace <w>]`, `hl use remove xcode --mac`},
	"use list":   {`hl use list`, `hl use list`},
	"screen":     {`hl screen list   |   hl screen shot --app <name> [--window <id>]   (a native window saved as a PNG, its path printed; for a web page use hl browser, for a phone app hl sim)`, `hl screen shot --app Finder`},
	"sim":        {`hl sim list   |   hl sim boot <device>   |   hl sim install <app>   |   hl sim launch <bundle id|package>   |   hl sim tree [--json]   |   hl sim tap (<x> <y> | --label L | --id I | --text T)   |   hl sim swipe (up|down|left|right | <x1> <y1> <x2> <y2>)   |   hl sim type "<text>"   |   hl sim button <name>   |   hl sim rotate <portrait|left|right|upsidedown>   |   hl sim screenshot <file.png>   |   hl sim shutdown   (a command after boot takes --device <name|id> when you hold more than one)`, `hl sim tap --label "General"`},
	"version":    {`hl version`, `hl version`},
	"handoff":    {`hl handoff slow [--since <span>] [--agent <name>]`, `hl handoff slow --since 7d`},
	"chat":       {`hl chat search "<text>" [--with <name>] [--since <span>] [--limit N]`, `hl chat search "nav order" --with handline --since 2d`},
	"task":       {`hl task <new|show|list|board|done|handoff|pass|back|edit|comment|begin|start|reopen|release|discard|accept|reject|queue|draft|plan|merge|request> ...`, `hl task show T12`}, // local-ref-ok: the usage example
	"task new": {`hl task new --assign <agent> -t <span> "<title>" "<body>" [--owner <agent>] [--parent T<n> | --no-parent (while your task is a request, a new task hangs under it unless you say this)] [--request T<n> (serve T<n>'s founder request)] [--blocked-by T<n>] [--priority 1-5] [--estimate N] [--expects report|decision] [--tag T] [--file PATH] [--draft (kept for its assignee's plan; nobody is asked until it is queued)]`,
		`hl task new --assign pc -t 2h "Fix the sidebar" "The badge overlaps the name"`},
	"task show":  {`hl task show T<n>`, `hl task show T12`},
	"task list":  {`hl task list [--assign <agent>] [--owner <agent>] [--state <state>] [--tag T] [--all] [--everyone]`, `hl task list --state doing`},
	"task board": {`hl task board [--everyone]`, `hl task board`},
	"task done":  {`hl task done T<n> "<evidence>" [--request <msg>] [--repo PATH] [--commit SHA] [--shot FILE]`, `hl task done T12 "PR #400 (sha 1a2b3c4), tests green"`},
	"task handoff": {`hl task handoff T<n> <agent> "<note>" [--file PATH] [--attach FILE] [--checked "<lessons the fresh check kept>" --misses "<what it found>" --tests pass|fail] (gives the task away; the founder's decision is hl task comment T<n> add "..." --turn human;` +
		` a story's sub-task handed to the land_by agent waits for the founder's OK first, the note and --attach files, pictures or an MP4/MOV video with a same-named picture as its poster, its result)`, `hl task handoff T12 builder "the parser half is yours"`},
	"task turn": {`hl task turn ... is gone: hl task comment T<n> add "<text>" --turn <agent|human> [--done "<evidence>"]`,
		`hl task comment T12 add "Ship to beta now, or after the fix? I recommend now." --turn human`},
	"task pass": {`hl task pass T<n> ["<note>"] (your turn is done; the task goes back to the one below)`, `hl task pass T12 "reviewed: fine"`},
	"task back": {`hl task back T<n> "<why>" (sends the task back to the one below, with why)`, `hl task back T12 "the test misses the empty list"`},
	"task edit": {`hl task edit T<n> [--assign <agent>] [--owner <agent>] [--title T] [--body B | --body-file PATH] [--priority 1-5] [--estimate N] [--blocked-by T<m>] [--unblock T<m>] [--timeout <span>] [--tag T] [--untag T] [--pr N [--repo owner/name]] [--parent T<m> | --no-parent]`,
		`hl task edit T12 --blocked-by T13`},
	"task comment": {`hl task comment T<n> add "<text>" [--turn <agent|human> [--done "<evidence>"]] [--reply <id>] [--attach FILE] [--now]   |   list   |   edit <id> "<text>"   |   delete <id>` +
		` (--turn hands the task to them with this comment, your turn waiting below; --done: their pass closes the task; "@human ..." first is news for the founder, no turn)`,
		`hl task comment T12 add "Ship to beta now, or after the fix? I recommend now." --turn human`},
	"task begin":       {`hl task begin "<title>" --chat <msg> [--why W] [--priority 1-5] [--expects report|decision]`, `hl task begin "Answer the founder" --chat msg_1a2b3c4`},
	"task start":       {`hl task start T<n>`, `hl task start T12`},
	"task reopen":      {`hl task reopen T<n> "<why>"`, `hl task reopen T12 "the badge still overlaps on the phone"`},
	"task release":     {`hl task release T<n> "<why>"`, `hl task release T12 "blocked on the founder"`},
	"task wait":        {`hl task wait T<n> (--on <vendor>:<target> | --pr <n> [--repo owner/name]) [--until ci|closed|landed|done] [--then done] [-t <span>] (--until landed --then done: the merge closes the task with its commit)`, `hl task wait T12 --pr 412 --until landed --then done`}, // local-ref-ok
	"task discard":     {`hl task discard T<n> "<why>"   |   hl task discard T<n> T<m> --why "<why>" [--why-each T<m>="<why>"]`, `hl task discard T12 "duplicate of T9"`},
	"task request":     {`hl task request <msg> ["<title>"] [--owner <agent>]`, `hl task request msg_1a2b3c4 "Rework the sidebar"`},
	"task note":        {`hl task note T<n> "<text>"`, `hl task note T12 "the phone needs it too"`},
	"task queue":       {`hl task queue T<n> [T<m> ...]`, `hl task queue T12`},
	"task draft":       {`hl task draft T<n> [T<m> ...]`, `hl task draft T12`},
	"task plan":        {`hl task plan T<n> (--file PATH | --ok | --not-ok)`, `hl task plan T12 --file plan.md`}, // local-ref-ok: the usage example
	"task merge":       {`hl task merge T<n> T<m> [...]`, `hl task merge T12 T13`},
	"hook":             {`hl hook <event>`, `hl hook Stop`},
	"propose":          {`hl propose <add|retire|rename|team|context|settings|sandbox|schedule|bundle|run|list|withdraw> ...`, `hl propose settings web effort=high --why "its reviews miss bugs"`},
	"propose add":      {`hl propose add <workspace/name> --repo PATH --why "<why>" [--team T] [--model M] [--effort E] [--vendor V]`, `hl propose add shop/sea --repo /code/sea --why "split the storefront work"`},
	"propose retire":   {`hl propose retire <agent> --why "<why>" (refused when applied while a worktree of the agent holds work)`, `hl propose retire old-agent --why "its work moved to new-agent"`},
	"propose rename":   {`hl propose rename <agent> --to <new name> --why "<why>" [--team T]`, `hl propose rename shop/sea --to ocean --why "clearer name"`},
	"propose team":     {`hl propose team <manager> --member <name> ... --repo PATH --why "<why>"`, `hl propose team shop --member sea --member shore --repo /code/shop --why "a storefront team"`},
	"propose context":  {`hl propose context <agent> [--role-file PATH] [--base-file PATH] [--rules-file PATH (its workspace's rules)] --why "<why>"`, `hl propose context shop/sea --role-file role.md --why "its role grew"`},
	"propose schedule": {`hl propose schedule <agent> --cron "<expr>" --message "<text>" [--was "<old cron>" [--was-message "<old text>"]] | --remove --was "<cron>" --why "<why>"`, `hl propose schedule shop/sea --cron "0 9 * * 1" --message "Review last week's open PRs" --why "a weekly review nobody remembers"`},
	"propose sandbox":  {`hl propose sandbox <host <name>|command <command>|write <folder>|cert> --why "<why>" (one entry for your sandbox: a host to reach, a command run outside it, a folder to write, or the macOS certificate check Go-built tools need; the founder's OK adds it and keeps the rest)`, `hl propose sandbox host pypi.org --why "the tests install their Python tools"`},
	"propose run":      {`hl propose run "<command>" [--cwd PATH] --why "<why>" (the founder's approve runs it once in a terminal window in PATH, default here; you are told how it ended)`, `hl propose run "rm -r build" --why "frees 2 GB"`},
	"propose list":     {`hl propose list [--json] [--all]`, `hl propose list`},
	"propose withdraw": {`hl propose withdraw <proposal id>`, `hl propose withdraw 1a2b3c4d`},
	"init":             {`hl init`, `hl init`},
	"launch":           {launchUsage, `hl launch shop/sea`},
	"device":           {`hl device pair [--name N] | hl device join <link>`, `hl device pair --name phone`},
	"node":             {`hl node promote (run on one of the owner's machines that joined: it becomes the master)`, `hl node promote`},
	"workspace":        {`hl workspace <new|rename|delete> ... (the founder's; an agent proposes a team)`, `hl workspace new shop --folder ~/code/shop`},
	"workspace new":    {`hl workspace new [<name>] [--folder PATH] (default: this folder's git checkout; no name: the folder's)`, `hl workspace new shop --folder ~/code/shop`},
	"workspace rename": {`hl workspace rename <workspace> --to <new name>`, `hl workspace rename shop --to store`},
	"workspace delete": {`hl workspace delete <workspace> [--worktree save|force] (retires its agents, lead last; the project folder stays)`, `hl workspace delete shop --worktree save`},
	"dev":              {`hl dev cluster up|down|status [--runners N] [--worktree PATH]`, `hl dev cluster up --runners 2`},
	"dev cluster":      {`hl dev cluster up|down|status [--runners N] [--worktree PATH]`, `hl dev cluster up --runners 2`},
}

// usageOf is the form and example for args to verb: its subverb's when it has one, else its own.
func usageOf(verb string, args []string) ([2]string, bool) {
	if len(args) > 0 {
		if u, ok := usages[verb+" "+args[0]]; ok {
			return u, true
		}
	}
	u, ok := usages[verb]
	return u, ok
}
