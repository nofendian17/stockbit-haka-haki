package helpers

import "fmt"

// FormatRupiah formats a number as Indonesian Rupiah currency
func FormatRupiah(amount float64) string {
	// Convert to integer for formatting
	value := int64(amount)

	// Handle negative numbers
	negative := value < 0
	if negative {
		value = -value
	}

	// Convert to string and add thousand separators
	str := fmt.Sprintf("%d", value)
	length := len(str)

	if length <= 3 {
		if negative {
			return fmt.Sprintf("Rp -%s", str)
		}
		return fmt.Sprintf("Rp %s", str)
	}

	// Build the formatted string with dots as thousand separators
	var result string
	for i, digit := range str {
		if i > 0 && (length-i)%3 == 0 {
			result += "."
		}
		result += string(digit)
	}

	if negative {
		return fmt.Sprintf("Rp -%s", result)
	}
	return fmt.Sprintf("Rp %s", result)
}
