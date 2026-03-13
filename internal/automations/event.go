package automations

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Event represents a trigger event that starts an automation workflow.
type Event struct {
	Type       string          `json:"type"`       // "github_webhook" or "cron"
	Name       string          `json:"name"`       // automation name that matched
	Timestamp  time.Time       `json:"timestamp"`
	Raw        json.RawMessage `json:"raw,omitempty"` // raw event payload (webhook body)
	Properties map[string]any  `json:"properties,omitempty"`
}

// ExpandTemplate replaces {{event.*}} placeholders in a template string
// using the event's properties. Properties are accessed with dot notation:
// {{event.pull_request.html_url}} navigates nested maps.
func (e Event) ExpandTemplate(tmpl string) string {
	result := tmpl
	for k, v := range e.flattenedProperties("event") {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
	}
	return result
}

// flattenedProperties returns a flat map of "prefix.key.subkey" -> value
// for use in template expansion.
func (e Event) flattenedProperties(prefix string) map[string]any {
	result := make(map[string]any)
	flattenMap(prefix, e.Properties, result)
	return result
}

func flattenMap(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := prefix + "." + k
		switch val := v.(type) {
		case map[string]any:
			flattenMap(key, val, out)
		default:
			out[key] = val
		}
	}
}
