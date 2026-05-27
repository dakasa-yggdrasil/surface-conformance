package conformance

import (
	"encoding/json"
)

// jsonFinding is the on-the-wire JSON projection of a Finding. We use a
// dedicated type so the Severity field renders as a stable string instead of
// the Go iota int.
type jsonFinding struct {
	Invariant  Invariant `json:"invariant"`
	Severity   string    `json:"severity"`
	File       string    `json:"file"`
	Line       int       `json:"line,omitempty"`
	Message    string    `json:"message"`
	Suggestion string    `json:"suggestion,omitempty"`
}

func marshalIndentJSON(findings []Finding) ([]byte, error) {
	out := make([]jsonFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, jsonFinding{
			Invariant:  f.Invariant,
			Severity:   f.Severity.String(),
			File:       f.File,
			Line:       f.Line,
			Message:    f.Message,
			Suggestion: f.Suggestion,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}
