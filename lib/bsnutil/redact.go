package bsnutil

import "regexp"

// RedactedBSN replaces every BSN that Redact removes.
const RedactedBSN = "[redacted BSN]"

var digitRun = regexp.MustCompile(`[0-9]+`)

// Redact replaces every BSN in s with RedactedBSN, for text that may echo one (an upstream error
// message, a request URL). A BSN is taken to be a run of exactly 9 digits that passes the
// elfproef. Shorter or longer numbers, such as an 8-digit URA or AGB code, are kept, so the text
// stays useful for finding out what went wrong.
func Redact(s string) string {
	return digitRun.ReplaceAllStringFunc(s, func(digits string) string {
		if ValidElfproef(digits) {
			return RedactedBSN
		}
		return digits
	})
}
