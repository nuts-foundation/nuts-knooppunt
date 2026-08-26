package bsnutil

// ValidElfproef reports whether bsn passes the Dutch "elfproef" (11-proof), the
// checksum every valid BSN satisfies.
//
// A BSN is exactly 9 digits. Weighting the digits left-to-right by 9, 8, 7, 6,
// 5, 4, 3, 2 and the last digit by -1, the weighted sum must be a non-zero
// multiple of 11. (The last digit is subtracted rather than added, per RvIG.)
//
// Note: passing the elfproef does not make a BSN a real or issued number. For
// synthetic test data the BSN must additionally be present in the published
// RvIG test-BSN set; ValidElfproef only rules out typos and structurally
// invalid numbers.
func ValidElfproef(bsn string) bool {
	if len(bsn) != 9 {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		c := bsn[i]
		if c < '0' || c > '9' {
			return false
		}
		digit := int(c - '0')
		weight := 9 - i
		if i == 8 {
			// The final digit carries weight -1.
			weight = -1
		}
		sum += weight * digit
	}
	// "000000000" weights to 0, which is a multiple of 11 but not a valid BSN.
	return sum != 0 && sum%11 == 0
}
