// Package linebuf is how long one scanned line may be.
// The model stream, the .env reader, and the line REPL share it.
package linebuf

const (
	Start = 64 * 1024
	Max   = 1024 * 1024
)
