package common

import "strings"

// MacRoman's upper half, in byte order. The command-key control character uses
// the same visible Unicode representation as the existing XML conversion.
var macRomanUpper = []rune("ÄÅÇÉÑÖÜáàâäãåçéèêëíìîïñóòôöõúùûü†°¢£§•¶ß®©™´¨≠ÆØ∞±≤≥¥µ∂∑∏π∫ªºΩæø¿¡¬√ƒ≈∆«»… ÀÃÕŒœ–—“”‘’÷◊ÿŸ⁄€‹›ﬁﬂ‡·‚„‰ÂÊÁËÈÍÎÏÌÓÔÒÚÛÙıˆ˜¯˘˙˚¸˝˛ˇ")

// DecodeMacRoman converts legacy Macintosh text to UTF-8. It preserves the
// native HyperCard reader's visible command-key glyph and normalizes CR/CRLF
// line endings to LF. The input bytes are never modified.
func DecodeMacRoman(data []byte) string {
	var out strings.Builder
	out.Grow(len(data))
	for _, c := range data {
		switch {
		case c >= 128:
			out.WriteRune(macRomanUpper[c-128])
		case c == 0x11:
			out.WriteRune('⌘')
		default:
			out.WriteByte(c)
		}
	}
	text := strings.ReplaceAll(out.String(), "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
