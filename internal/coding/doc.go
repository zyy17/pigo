// Package coding is the product surface, the role Pi calls pi-coding-agent.
//
// It confines file paths to one workspace, offers the coding tools, discovers
// skills, and wires those tools into an agent session. The session can append
// the transcript to a JSONL file and can append a compaction summary as
// the leaf of that file. It does not speak HTTP.
package coding
