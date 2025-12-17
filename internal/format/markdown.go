package format

import (
	"regexp"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ParseResult contains plain text and message entities
type ParseResult struct {
	Text     string
	Entities []tgbotapi.MessageEntity
}

// UTF16Len calculates the UTF-16 length of a string
// This is required because Telegram uses UTF-16 code units for entity offsets/lengths
func UTF16Len(s string) int {
	length := 0
	for _, b := range []byte(s) {
		if (b & 0xc0) != 0x80 {
			if b >= 0xf0 {
				length += 2 // Non-BMP characters (surrogate pairs)
			} else {
				length += 1
			}
		}
	}
	return length
}

// ParseMarkdown parses standard Markdown and converts it to Telegram message entities
// Supported formats:
// - **bold** or __bold__ -> bold
// - *italic* or _italic_ -> italic
// - `code` -> code
// - # Header -> bold (header converted to bold)
func ParseMarkdown(text string) ParseResult {
	var entities []tgbotapi.MessageEntity
	result := text

	// Pattern for headers: # Header at the start of a line
	// Must be processed first as it affects entire lines
	headerRe := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)$`)
	result = headerRe.ReplaceAllStringFunc(result, func(m string) string {
		submatch := headerRe.FindStringSubmatch(m)
		if len(submatch) >= 3 {
			return "**" + submatch[2] + "**" // Convert header to bold
		}
		return m
	})

	// Process patterns in order of specificity
	// 1. Bold: **text** or __text__
	// 2. Italic: *text* or _text_ (single)
	// 3. Code: `code`

	// Bold pattern: **text** (must be processed before italic)
	boldRe := regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	// Code pattern: `code`
	codeRe := regexp.MustCompile("`([^`]+?)`")

	// Process bold
	for {
		loc := boldRe.FindStringSubmatchIndex(result)
		if loc == nil {
			break
		}

		fullStart, fullEnd := loc[0], loc[1]
		var innerText string
		if loc[2] != -1 { // **text**
			innerText = result[loc[2]:loc[3]]
		} else { // __text__
			innerText = result[loc[4]:loc[5]]
		}

		// Calculate UTF-16 offset before this position
		offset := UTF16Len(result[:fullStart])
		innerLen := UTF16Len(innerText)

		entities = append(entities, tgbotapi.MessageEntity{
			Type:   "bold",
			Offset: offset,
			Length: innerLen,
		})

		// Remove the markers but keep the inner text
		result = result[:fullStart] + innerText + result[fullEnd:]
	}

	// Process code (before italic to avoid conflicts)
	for {
		loc := codeRe.FindStringSubmatchIndex(result)
		if loc == nil {
			break
		}

		fullStart, fullEnd := loc[0], loc[1]
		innerText := result[loc[2]:loc[3]]

		offset := UTF16Len(result[:fullStart])
		innerLen := UTF16Len(innerText)

		entities = append(entities, tgbotapi.MessageEntity{
			Type:   "code",
			Offset: offset,
			Length: innerLen,
		})

		result = result[:fullStart] + innerText + result[fullEnd:]
	}

	// Process italic - more careful pattern to avoid matching ** or __
	// Use a simpler approach: find *text* where text doesn't contain *
	singleStarRe := regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	singleUnderRe := regexp.MustCompile(`(?:^|[^_])_([^_]+?)_(?:[^_]|$)`)

	// Process *italic*
	processItalic := func(re *regexp.Regexp, marker string) {
		searchStart := 0
		for searchStart < len(result) {
			loc := re.FindStringSubmatchIndex(result[searchStart:])
			if loc == nil {
				break
			}

			// Adjust for searchStart
			for i := range loc {
				if loc[i] != -1 {
					loc[i] += searchStart
				}
			}

			// Find the actual marker positions
			innerText := result[loc[2]:loc[3]]
			// Find where *innerText* actually is
			markerPattern := regexp.MustCompile(regexp.QuoteMeta(marker) + regexp.QuoteMeta(innerText) + regexp.QuoteMeta(marker))
			markerLoc := markerPattern.FindStringIndex(result[searchStart:])
			if markerLoc == nil {
				searchStart = loc[1]
				continue
			}
			markerLoc[0] += searchStart
			markerLoc[1] += searchStart

			offset := UTF16Len(result[:markerLoc[0]])
			innerLen := UTF16Len(innerText)

			entities = append(entities, tgbotapi.MessageEntity{
				Type:   "italic",
				Offset: offset,
				Length: innerLen,
			})

			result = result[:markerLoc[0]] + innerText + result[markerLoc[1]:]
			searchStart = markerLoc[0] + len(innerText)
		}
	}

	processItalic(singleStarRe, "*")
	processItalic(singleUnderRe, "_")

	// Sort entities by offset (Telegram requires this)
	for i := 0; i < len(entities); i++ {
		for j := i + 1; j < len(entities); j++ {
			if entities[j].Offset < entities[i].Offset {
				entities[i], entities[j] = entities[j], entities[i]
			}
		}
	}

	return ParseResult{
		Text:     strings.TrimRight(result, " \n"),
		Entities: entities,
	}
}
