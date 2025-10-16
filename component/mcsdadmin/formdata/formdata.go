package formdata

func FilterEmpty(multiStrings []string) []string {
	out := make([]string, 0, len(multiStrings))
	for _, str := range multiStrings {
		if str != "" {
			out = append(out, str)
		}
	}
	return out
}
