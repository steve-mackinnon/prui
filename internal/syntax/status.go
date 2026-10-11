package syntax

// Status is bounded diagnostic metadata, never a raw lexer error or source text.
type Status string

const (
	Ready              Status = "ready"
	Unsupported        Status = "unsupported"
	ByteLimit          Status = "byte_limit"
	SpanLimit          Status = "span_limit"
	InvalidText        Status = "invalid_text"
	Timeout            Status = "timeout"
	Canceled           Status = "canceled"
	LexerError         Status = "lexer_error"
	InvalidOffsets     Status = "invalid_offsets"
	InventoryTimeLimit Status = "inventory_time_limit"
	InventoryByteLimit Status = "inventory_byte_limit"
	InventorySpanLimit Status = "inventory_span_limit"
	StorageLimit       Status = "storage_limit"
	Unknown            Status = "unknown"
)

type FileStatus struct {
	Old Status `json:"old"`
	New Status `json:"new"`
}

// Description deliberately does not echo arbitrary persisted strings.
func (s Status) Description() string {
	switch s {
	case Ready:
		return "available"
	case Unsupported:
		return "unsupported file type"
	case ByteLimit:
		return "file exceeds syntax size limit"
	case SpanLimit:
		return "file exceeds syntax token limit"
	case InvalidText:
		return "source is not valid UTF-8"
	case Timeout:
		return "capture timed out"
	case Canceled:
		return "capture canceled"
	case LexerError:
		return "lexer failed"
	case InvalidOffsets:
		return "lexer offsets did not match source"
	case InventoryTimeLimit:
		return "comparison syntax time budget exhausted"
	case InventoryByteLimit:
		return "comparison syntax size budget exhausted"
	case InventorySpanLimit:
		return "comparison syntax token budget exhausted"
	case StorageLimit:
		return "syntax metadata omitted by storage limit"
	default:
		return "saved syntax metadata empty; reason not recorded"
	}
}

func (s FileStatus) Warning() bool {
	return s.Old != Ready && s.Old != Unsupported || s.New != Ready && s.New != Unsupported
}

func (s FileStatus) Description() string {
	if s.Old == s.New {
		return s.Old.Description()
	}
	if s.Old == Ready {
		return "NEW: " + s.New.Description()
	}
	if s.New == Ready {
		return "OLD: " + s.Old.Description()
	}
	return "OLD: " + s.Old.Description() + "; NEW: " + s.New.Description()
}

// DroppedStatuses copies capture results before recording storage omission.
// A pre-existing failure reason remains more useful than the later omission.
func DroppedStatuses(statuses map[string]FileStatus) map[string]FileStatus {
	if statuses == nil {
		return nil
	}
	result := make(map[string]FileStatus, len(statuses))
	for id, status := range statuses {
		if status.Old == Ready {
			status.Old = StorageLimit
		}
		if status.New == Ready {
			status.New = StorageLimit
		}
		result[id] = status
	}
	return result
}
