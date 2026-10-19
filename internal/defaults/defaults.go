// Package defaults holds the built-in model, the API root, and the
// long-line scanner size. config and coding both need the first two,
// and neither imports the other.
package defaults

const (
	Model     = "gpt-5.6-luna"
	BaseURL   = "https://api.openai.com/v1"
	LineStart = 64 * 1024
	LineMax   = 1024 * 1024
)
