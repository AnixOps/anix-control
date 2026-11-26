package meshregion

import (
	"encoding/json"
	"time"
)

// The region change suggestion a node's kernel reports in its status
// (appendix D.6, D.9.2: region_probe.suggestion). It is kept as part of
// the stored status payload, shown to the administrator as
// region_suggestion, and never acted on: a move needs a request and an
// approval.

// RegionSuggestion is region_probe.suggestion of a status.
type RegionSuggestion struct {
	// Region is the code of the suggested region.
	Region string `json:"region"`
	// CurrentMedianMS and SuggestedMedianMS are the median round trips of
	// the current and the suggested region, in whole milliseconds.
	CurrentMedianMS   int64 `json:"current_median_ms"`
	SuggestedMedianMS int64 `json:"suggested_median_ms"`
	// Since is when the condition began to hold (RFC 3339).
	Since time.Time `json:"since"`
}

// parseSuggestion reads a suggestion: nil for none (absent or null), an
// error for anything but a region code, two whole non-negative millisecond
// numbers and an RFC 3339 time.
func parseSuggestion(raw json.RawMessage) (*RegionSuggestion, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var shape struct {
		Region            string      `json:"region"`
		CurrentMedianMS   json.Number `json:"current_median_ms"`
		SuggestedMedianMS json.Number `json:"suggested_median_ms"`
		Since             string      `json:"since"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return nil, invalid("payload: region_probe.suggestion is null or an object of region, current_median_ms, suggested_median_ms and since")
	}
	if !codePattern.MatchString(shape.Region) {
		return nil, invalid("payload: region_probe.suggestion.region is a region code")
	}
	current, err := shape.CurrentMedianMS.Int64()
	suggested, err2 := shape.SuggestedMedianMS.Int64()
	if err != nil || err2 != nil || current < 0 || suggested < 0 {
		return nil, invalid("payload: region_probe.suggestion median times are whole milliseconds, 0 or more")
	}
	since, err := time.Parse(time.RFC3339, shape.Since)
	if err != nil {
		return nil, invalid("payload: region_probe.suggestion.since is an RFC 3339 time")
	}
	return &RegionSuggestion{Region: shape.Region, CurrentMedianMS: current, SuggestedMedianMS: suggested, Since: since}, nil
}

// regionProbeSuggestion reads the suggestion of a status payload; an error
// when region_probe or its suggestion is malformed.
func regionProbeSuggestion(payload []byte) (*RegionSuggestion, error) {
	var top struct {
		RegionProbe json.RawMessage `json:"region_probe"`
	}
	if err := json.Unmarshal(payload, &top); err != nil {
		return nil, invalid("payload: a JSON object")
	}
	if len(top.RegionProbe) == 0 || string(top.RegionProbe) == "null" {
		return nil, nil
	}
	var probe struct {
		Suggestion json.RawMessage `json:"suggestion"`
	}
	if err := json.Unmarshal(top.RegionProbe, &probe); err != nil {
		return nil, invalid("payload: region_probe is null or an object")
	}
	return parseSuggestion(probe.Suggestion)
}

// suggestionOf is the suggestion of a stored status payload, nil for none
// or a malformed one (a node's report is shown as it is, not refused).
func suggestionOf(payload string) *RegionSuggestion {
	suggestion, err := regionProbeSuggestion([]byte(payload))
	if err != nil {
		return nil
	}
	return suggestion
}
