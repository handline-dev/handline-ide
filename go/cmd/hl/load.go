package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/handline/handline/go/internal/hlclient"
)

// loadCmd is hl load: what each agent's processes use right now, from load.get.
func loadCmd(c *hlclient.Client, args []string) (int, bool) {
	p, ok := parse(args, []string{"--agent"}, nil, nil)
	if !ok || len(p.pos) != 0 {
		return 0, false
	}
	params := map[string]any{}
	if a, _ := p.opt("--agent"); a != "" {
		params["agent"] = a
	}
	got, err := c.Call("load.get", params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hl load: %v\n", err)
		return 1, true
	}
	if len(got) == 0 {
		fmt.Println("The daemon has not measured the processes yet.")
		return 0, true
	}
	fmt.Print(loadText(got))
	return 0, true
}

func loadBytes(n int64) string {
	const mb = 1 << 20
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n/mb)
}

func fnum(o obj, k string) float64 {
	f, _ := o[k].(float64)
	return f
}

func procText(p obj) string {
	cmd := str(p, "command")
	if i := strings.IndexByte(cmd, ' '); i > 0 && len(cmd) > 40 {
		cmd = cmd[:i]
	}
	if len(cmd) > 40 {
		cmd = cmd[:40]
	}
	end := ""
	if r, _ := p["running"].(bool); !r {
		end = " (ended)"
	}
	return fmt.Sprintf("%s %.0f%%/%s%s", cmd, fnum(p, "cpu_pct"), loadBytes(num(p, "rss_bytes")), end)
}

// loadText is the view as text: the machine, then one line per agent with its three heaviest processes.
func loadText(got obj) string {
	var b strings.Builder
	m, _ := got["machine"].(obj)
	fmt.Fprintf(&b, "machine: %d cores, load %.1f (%.2f per core), %s free of %s memory, %s disk free\n", num(m, "cores"), fnum(m, "load_1m"),
		fnum(m, "load_per_core"), loadBytes(num(m, "ram_free_bytes")), loadBytes(num(m, "ram_total_bytes")), loadBytes(num(m, "disk_free_bytes")))
	tw := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tCPU\tMEMORY\tPROCS\tHEAVIEST")
	for _, a := range list(got, "agents") {
		var top []string
		for _, p := range list(a, "top") {
			top = append(top, procText(p))
		}
		fmt.Fprintf(tw, "%s\t%.0f%%\t%s\t%d\t%s\n", str(a, "agent"), fnum(a, "cpu_pct"), loadBytes(num(a, "rss_bytes")), num(a, "processes"), strings.Join(top, " · "))
	}
	u, _ := got["unowned"].(obj)
	var top []string
	for i, p := range list(u, "top") {
		if i == 3 {
			break
		}
		top = append(top, procText(p))
	}
	fmt.Fprintf(tw, "(no agent)\t%.0f%%\t%s\t%d\t%s\n", fnum(u, "cpu_pct"), loadBytes(num(u, "rss_bytes")), num(u, "processes"), strings.Join(top, " · "))
	tw.Flush()
	s, _ := got["sampling"].(obj)
	fmt.Fprintf(&b, "every %ds; a sample takes %.1f ms\n", num(got, "every_s"), fnum(s, "mean_ms"))
	return b.String()
}
