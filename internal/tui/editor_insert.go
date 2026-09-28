package tui

// insertEditorText inserts at a rune offset and returns the updated cursor.
func insertEditorText(draft string, cursor int, text string) (string, int) {
	runes, inserted := []rune(draft), []rune(text)
	cursor = min(max(cursor, 0), len(runes))
	updated := make([]rune, len(runes)+len(inserted))
	copy(updated, runes[:cursor])
	copy(updated[cursor:], inserted)
	copy(updated[cursor+len(inserted):], runes[cursor:])
	return string(updated), cursor + len(inserted)
}
