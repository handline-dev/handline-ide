// Package hosts picks the terminal hosts of the OS a binary is built for, the way package vendors links the agent CLIs: a binary that
// opens terminals imports it, and term.ForName then knows those hosts by name. Default is the host used when nothing names one.
package hosts
