package core

import "strings"

// AudioFormatToMediaType converts a short audio format string to a full MIME type.
func AudioFormatToMediaType(format string) string {
	switch strings.ToLower(format) {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "webm":
		return "audio/webm"
	default:
		// If it looks like a full MIME type already, pass through
		if strings.Contains(format, "/") {
			return format
		}
		return "audio/" + format
	}
}
