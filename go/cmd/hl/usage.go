package main

// usages is each verb's form and one example, printed when its arguments are refused: "<verb> <sub>" first, then "<verb>".
var usages = map[string][2]string{
	"say":      {`hl say <target> "<text>" [--parent <msg>] [--file PATH] [--attach FILE] (chat, queued until the target takes it; on a task, write a comment)`, `hl say builder "the build machine is free again"`},
	"read":     {`hl read <msg> [--wait] [--timeout <span>]`, `hl read msg_1a2b3c4`},
	"inbox":    {`hl inbox [--all] [--wait] [--timeout <span>]`, `hl inbox --all`},
	"withdraw": {`hl withdraw <msg> [--reason "<why>"]`, `hl withdraw msg_1a2b3c4 --reason "sent to the wrong agent"`},
	"progress": {`hl progress [--done N --of M] "<done: ... left: ...>"`, `hl progress --done 3 --of 5 "wiring the gate"`},
	"load":     {`hl load [--agent <name>]`, `hl load`},
	"team":     {`hl team [--agent <name>]`, `hl team`},
	"use":      {`hl use <heavy|simulator|browser> -- <command...>   |   hl use <heavy|simulator|browser> --hold   |   hl use --release [<heavy|simulator|browser>]`, `hl use browser -- npx playwright test`},
	"version":  {`hl version`, `hl version`},
	"handoff":  {`hl handoff slow [--since <span>] [--agent <name>]`, `hl handoff slow --since 7d`},
	"chat":     {`hl chat search "<text>" [--with <name>] [--since <span>] [--limit N]`, `hl chat search "nav order" --with handline --since 2d`},
	"task":     {`hl task <new|show|list|board|done|handoff|pass|back|edit|comment|begin|start|reopen|release|discard|accept|reject|queue|draft|merge|request> ...`, `hl task show T12`},
	"task new": {`hl task new --assign <agent> -t <span> "<title>" "<body>" [--owner <agent>] [--parent T<n> | --no-parent (while your task is a request, a new task hangs under it unless you say this)] [--blocked-by T<n>] [--priority 1-5] [--estimate N] [--expects report|decision] [--tag T] [--file PATH]`,
		`hl task new --assign pc -t 2h "Fix the sidebar" "The badge overlaps the name"`},
	"task show":    {`hl task show T<n>`, `hl task show T12`},
	"task list":    {`hl task list [--assign <agent>] [--owner <agent>] [--state <state>] [--tag T] [--all] [--everyone]`, `hl task list --state doing`},
	"task board":   {`hl task board [--everyone]`, `hl task board`},
	"task done":    {`hl task done T<n> "<evidence>" [--request <msg>] [--repo PATH] [--commit SHA] [--shot FILE]`, `hl task done T12 "PR #400 (sha 1a2b3c4), tests green"`},
	"task handoff": {`hl task handoff T<n> <agent> "<note>" [--file PATH] (gives the task away; the founder's decision is hl task comment T<n> add "..." --turn human)`, `hl task handoff T12 builder "the parser half is yours"`},
	"task turn": {`hl task turn ... is gone: hl task comment T<n> add "<text>" --turn <agent|human> [--done "<evidence>"]`,
		`hl task comment T12 add "Ship to beta now, or after the fix? I recommend now." --turn human`},
	"task pass": {`hl task pass T<n> ["<note>"] (your turn is done; the task goes back to the one below)`, `hl task pass T12 "reviewed: fine"`},
	"task back": {`hl task back T<n> "<why>" (sends the task back to the one below, with why)`, `hl task back T12 "the test misses the empty list"`},
	"task edit": {`hl task edit T<n> [--assign <agent>] [--owner <agent>] [--title T] [--body B | --body-file PATH] [--priority 1-5] [--estimate N] [--blocked-by T<m>] [--unblock T<m>] [--timeout <span>] [--tag T] [--untag T] [--pr N] [--parent T<m> | --no-parent]`,
		`hl task edit T12 --blocked-by T13`},
	"task comment": {`hl task comment T<n> add "<text>" [--turn <agent|human> [--done "<evidence>"]] [--reply <id>] [--attach FILE] [--now]   |   list   |   edit <id> "<text>"   |   delete <id>` +
		` (--turn hands the task to them with this comment, your turn waiting below; --done: their pass closes the task; "@human ..." first is news for the founder, no turn)`,
		`hl task comment T12 add "Ship to beta now, or after the fix? I recommend now." --turn human`},
	"task memo":        {`hl task memo T<n> ... (the old name of hl task comment)`, `hl task comment T12 add "The fix also needs the phone"`},
	"task begin":       {`hl task begin "<title>" --chat <msg> [--why W] [--priority 1-5] [--expects report|decision]`, `hl task begin "Answer the founder" --chat msg_1a2b3c4`},
	"task start":       {`hl task start T<n>`, `hl task start T12`},
	"task reopen":      {`hl task reopen T<n> "<why>"`, `hl task reopen T12 "the badge still overlaps on the phone"`},
	"task release":     {`hl task release T<n> "<why>"`, `hl task release T12 "blocked on the founder"`},
	"task wait":        {`hl task wait T<n> (--on <vendor>:<target> | --pr <n> [--repo owner/name]) [--until ci|closed|done] [-t <span>]`, `hl task wait T12 --pr 412`},
	"task discard":     {`hl task discard T<n> "<why>"   |   hl task discard T<n> T<m> --why "<why>" [--why-each T<m>="<why>"]`, `hl task discard T12 "duplicate of T9"`},
	"task request":     {`hl task request <msg> ["<title>"] [--owner <agent>]`, `hl task request msg_1a2b3c4 "Rework the sidebar"`},
	"task note":        {`hl task note T<n> "<text>"`, `hl task note T12 "the phone needs it too"`},
	"task queue":       {`hl task queue T<n> [T<m> ...]`, `hl task queue T12`},
	"task draft":       {`hl task draft T<n> [T<m> ...]`, `hl task draft T12`},
	"task merge":       {`hl task merge T<n> T<m> [...]`, `hl task merge T12 T13`},
	"hook":             {`hl hook <event>`, `hl hook Stop`},
	"propose":          {`hl propose <add|retire|rename|team|context|schedule|bundle|list|withdraw> ...`, `hl propose list`},
	"propose add":      {`hl propose add <agent> --repo PATH --why "<why>" [--team T] [--model M] [--effort E] [--vendor V]`, `hl propose add shop.sea --repo /code/sea --why "split the storefront work"`},
	"propose retire":   {`hl propose retire <agent> --why "<why>" (refused when applied while a worktree of the agent holds work)`, `hl propose retire old.agent --why "its work moved to new.agent"`},
	"propose rename":   {`hl propose rename <agent> --to <new name> --why "<why>" [--team T]`, `hl propose rename shop.sea --to shop.ocean --why "clearer name"`},
	"propose team":     {`hl propose team <manager> --member <name> ... --repo PATH --why "<why>"`, `hl propose team shop --member sea --member shore --repo /code/shop --why "a storefront team"`},
	"propose context":  {`hl propose context <agent> [--role-file PATH] [--base-file PATH] --why "<why>"`, `hl propose context shop.sea --role-file role.md --why "its role grew"`},
	"propose schedule": {`hl propose schedule <agent> --cron "<expr>" --message "<text>" [--was "<old cron>" [--was-message "<old text>"]] | --remove --was "<cron>" --why "<why>"`, `hl propose schedule shop.sea --cron "0 9 * * 1" --message "Review last week's open PRs" --why "a weekly review nobody remembers"`},
	"propose list":     {`hl propose list [--json] [--all]`, `hl propose list`},
	"propose withdraw": {`hl propose withdraw <proposal id>`, `hl propose withdraw 1a2b3c4d`},
	"init":             {`hl init`, `hl init`},
	"launch":           {launchUsage, `hl launch shop.sea`},
	"sandbox-repo":     {sandboxRepoUsage, `hl sandbox-repo /code/sea npm`},
	"device":           {`hl device pair [--name N]`, `hl device pair --name phone`},
	"dev":              {`hl dev on|off|rebuild <team> [--worktree PATH] [--listen ADDR] [--fresh]`, `hl dev on shop`},
	"idle-background":  {`hl idle-background [--notice <span>] [--grace <span>] [--check <span>] [--clear]`, `hl idle-background --grace 10m`},
	"notice-digest":    {`hl notice-digest [<span>] [--agent <name>] [--clear]`, `hl notice-digest 2m --agent shop.sea`},
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
