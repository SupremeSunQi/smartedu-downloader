package resource

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxBasenameBytes = 180

var windowsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

func SafeFilename(title string) string {
	name := strings.Join(strings.Fields(title), " ")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	for strings.Contains(name, "..") {
		name = strings.ReplaceAll(name, "..", "")
	}
	name = strings.Trim(name, " .")
	if len(name) >= 4 && strings.EqualFold(name[len(name)-4:], ".pdf") {
		name = strings.Trim(name[:len(name)-4], " .")
	}
	if name == "" {
		name = "教材"
	}

	deviceCandidate := name
	if dot := strings.IndexRune(deviceCandidate, '.'); dot >= 0 {
		deviceCandidate = deviceCandidate[:dot]
	}
	if _, reserved := windowsReservedNames[strings.ToUpper(deviceCandidate)]; reserved {
		name = "_" + name
	}
	name = truncateUTF8(name, maxBasenameBytes)
	name = strings.TrimRight(name, " .")
	if name == "" {
		name = "教材"
	}
	return name + ".pdf"
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
