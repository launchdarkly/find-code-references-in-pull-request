package utils

func Dedupe(s []string) []string {
	if len(s) <= 1 {
		return s
	}
	keys := make(map[string]struct{}, len(s))
	ret := make([]string, 0, len(s))
	for _, elem := range s {
		if _, ok := keys[elem]; !ok {
			keys[elem] = struct{}{}
			ret = append(ret, elem)
		}
	}
	return ret
}

func SafeString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
