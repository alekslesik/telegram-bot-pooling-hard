package payments

import (
	"fmt"
	"regexp"
	"strings"
)

const externalOperationPrefix = "external_psp"

var (
	providerCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)
	providerRefPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{2,127}$`)
)

type ExternalCallback struct {
	ProviderCode string
	OperationRef string
	UserID       int64
	AmountMinor  int64
	Currency     string
}

func (c ExternalCallback) Validate() error {
	if !providerCodePattern.MatchString(strings.TrimSpace(c.ProviderCode)) {
		return fmt.Errorf("invalid provider code")
	}
	if !providerRefPattern.MatchString(strings.TrimSpace(c.OperationRef)) {
		return fmt.Errorf("invalid operation reference")
	}
	if c.UserID <= 0 {
		return fmt.Errorf("user id must be positive")
	}
	if c.AmountMinor <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if strings.TrimSpace(c.Currency) == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

func BuildExternalOperationID(providerCode, operationRef string) (string, error) {
	cb := ExternalCallback{
		ProviderCode: providerCode,
		OperationRef: operationRef,
		UserID:       1,
		AmountMinor:  1,
		Currency:     "N/A",
	}
	if err := cb.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s:%s", externalOperationPrefix, strings.TrimSpace(providerCode), strings.TrimSpace(operationRef)), nil
}
