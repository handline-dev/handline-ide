package runload

// nicer is whether a getpriority answer is LowNice or more: the raw call answers 20 less the niceness.
func nicer(got int) bool { return 20-got >= LowNice }

// startOf is not read here: a pid is lowered as the process table named it.
func startOf(int) (int64, bool) { return 0, false }
