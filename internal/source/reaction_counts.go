package source

import "encoding/json"

// ReactionCounts follows ReviewCommentReactions order and stays comparable.
// This metadata belongs only to the live comment overlay.
type ReactionCounts [8]int

type remoteReactionCounts map[string]int

func (c *remoteReactionCounts) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = remoteReactionCounts{}
	for _, token := range ReviewCommentReactions() {
		var count int
		if value, ok := raw[token]; ok && json.Unmarshal(value, &count) == nil && count > 0 && count <= 1_000_000_000 {
			(*c)[token] = count
		}
	}
	return nil
}

func boundedReactionCounts(raw map[string]int) ReactionCounts {
	var counts ReactionCounts
	for i, token := range ReviewCommentReactions() {
		if count := raw[token]; count > 0 && count <= 1_000_000_000 {
			counts[i] = count
		}
	}
	return counts
}
