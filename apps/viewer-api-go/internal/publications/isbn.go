package publications

import "strings"

func NormalizeISBN13(value string) string {
	digits := make([]rune, 0, 13)
	for _, r := range strings.TrimSpace(value) {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) != 13 {
		return ""
	}
	isbn := string(digits)
	if !strings.HasPrefix(isbn, "978") && !strings.HasPrefix(isbn, "979") {
		return ""
	}
	if !validISBN13Checksum(isbn) {
		return ""
	}
	return isbn
}

func validISBN13Checksum(isbn string) bool {
	if len(isbn) != 13 {
		return false
	}
	sum := 0
	for i := 0; i < 12; i++ {
		digit := int(isbn[i] - '0')
		if i%2 == 0 {
			sum += digit
		} else {
			sum += digit * 3
		}
	}
	check := (10 - (sum % 10)) % 10
	return check == int(isbn[12]-'0')
}
