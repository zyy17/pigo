// Package agent is the loop, the role Pi calls pi-agent-core.
//
// Prompt appends a user message, asks the model what to do next, runs any
// tool calls, and repeats until the model stops. When the context estimate
// passes the reserve, older messages are summarized first. The model
// transport is an ai.StreamFunc supplied by the caller. This package does
// not know about files, shells, or HTTP.
package agent
