// Package ai is the model transport, the role Pi calls pi-ai.
//
// It owns transcript messages, tool schemas, and streaming events, and it
// speaks HTTP to an OpenAI-compatible chat API. Tool execution and the
// decision to call the model again belong to package agent.
package ai
