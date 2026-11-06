package procload

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// envValue is the value of key in a KERN_PROCARGS2 buffer (macOS): an int32 argc, the executable path, NUL padding, argc arguments, then
// the environment as NUL-separated NAME=value strings. "" when the key is absent or the buffer is not that shape.
func envValue(buf []byte, key string) string {
	if len(buf) < 4 {
		return ""
	}
	argc := int(int32(binary.LittleEndian.Uint32(buf)))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0) // the executable path
	if i < 0 || argc < 0 {
		return ""
	}
	rest = bytes.TrimLeft(rest[i:], "\x00")
	for ; argc > 0; argc-- {
		i = bytes.IndexByte(rest, 0)
		if i < 0 {
			return ""
		}
		rest = rest[i+1:]
	}
	return envFrom(rest, key)
}

// envFrom is the value of key in NUL-separated NAME=value strings (also /proc/<pid>/environ).
func envFrom(env []byte, key string) string {
	prefix := key + "="
	for _, kv := range strings.Split(string(env), "\x00") {
		if v, ok := strings.CutPrefix(kv, prefix); ok {
			return v
		}
	}
	return ""
}
