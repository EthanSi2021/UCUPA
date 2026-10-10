package values

// Valid reports whether the Element ID is the canonical UCUPA lowercase
// RFC 9562 UUIDv4 representation with the RFC variant.
func (value ElementID) Valid() bool {
	return validUUIDv4(string(value))
}

func validUUIDv4(text string) bool {
	if len(text) != 36 ||
		text[8] != '-' || text[13] != '-' || text[18] != '-' || text[23] != '-' ||
		text[14] != '4' {
		return false
	}
	switch text[19] {
	case '8', '9', 'a', 'b':
	default:
		return false
	}
	for index, character := range []byte(text) {
		switch index {
		case 8, 13, 18, 23:
			continue
		}
		if character >= '0' && character <= '9' ||
			character >= 'a' && character <= 'f' {
			continue
		}
		return false
	}
	return true
}
