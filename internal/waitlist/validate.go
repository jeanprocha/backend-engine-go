package waitlist

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const maxEmailLen = 254

// NormalizeEmail alinha antes de gravar: minúsculas, sem espaço nas pontas —
// é sobre esse valor que a unicidade da tabela (waitlist_email_unique) roda,
// então "Nome@Ex.com " e "nome@ex.com" não podem virar duas linhas.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidateForInsert confere formato e tamanho do e-mail já normalizado.
func ValidateForInsert(email string) error {
	e := NormalizeEmail(email)
	if e == "" {
		return fmt.Errorf("email vazio")
	}
	if utf8.RuneCountInString(e) > maxEmailLen {
		return fmt.Errorf("email excede %d caracteres", maxEmailLen)
	}
	if _, err := mail.ParseAddress(e); err != nil {
		return fmt.Errorf("email inválido")
	}
	return nil
}
