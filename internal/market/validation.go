package market

import "strings"

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func ValidateStocks(stocks []Stock) error {
	seen := make(map[string]struct{}, len(stocks))
	for _, stock := range stocks {
		if strings.TrimSpace(stock.Name) == "" {
			return ValidationError{Message: "stock name must not be empty"}
		}
		if stock.Quantity < 0 {
			return ValidationError{Message: "stock quantity must not be negative"}
		}
		if _, ok := seen[stock.Name]; ok {
			return ValidationError{Message: "stock names must be unique"}
		}
		seen[stock.Name] = struct{}{}
	}

	return nil
}
