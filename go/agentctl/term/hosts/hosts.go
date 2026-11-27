// Package hosts links the terminal hosts, the way package vendors links the agent CLIs: a binary that opens terminals imports it, and
// term.ForName then knows those hosts by name. Default is the host used when nothing names one.
package hosts
