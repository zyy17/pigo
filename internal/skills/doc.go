// Package skills discovers SKILL.md files and formats the short list Pi puts
// in the system prompt.
//
// The agent loop never sees this package. The coding session discovers skills,
// and the model loads a full SKILL.md later with read_file.
package skills
