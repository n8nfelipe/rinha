package httpapi

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/felipe/rinha/internal/store"
)

var namePattern = regexp.MustCompile(`^[\p{L}][\p{L} .'-]+$`)
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func validateRecord(record store.Record) (store.Record, error) {
	record.Name = strings.Join(strings.Fields(record.Name), " ")
	record.Address = strings.TrimSpace(record.Address)
	record.Email = strings.ToLower(strings.TrimSpace(record.Email))
	record.Phone = normalizePhone(record.Phone)
	record.BirthDate = strings.TrimSpace(record.BirthDate)

	if utf8.RuneCountInString(record.Name) < 2 || utf8.RuneCountInString(record.Name) > 150 || !namePattern.MatchString(record.Name) {
		return store.Record{}, fmt.Errorf("name must contain 2..150 letters")
	}
	if utf8.RuneCountInString(record.Address) < 5 || utf8.RuneCountInString(record.Address) > 250 {
		return store.Record{}, fmt.Errorf("address must contain 5..250 characters")
	}
	if len(record.Phone) < 10 || len(record.Phone) > 15 {
		return store.Record{}, fmt.Errorf("phone must contain 10..15 digits")
	}
	parsedEmail, err := mail.ParseAddress(record.Email)
	if err != nil || parsedEmail.Address != record.Email || !strings.Contains(record.Email, "@") || len(record.Email) > 254 {
		return store.Record{}, fmt.Errorf("email is invalid")
	}
	birthDate, err := time.Parse("2006-01-02", record.BirthDate)
	if err != nil || birthDate.After(time.Now().UTC()) || birthDate.Before(time.Now().UTC().AddDate(-130, 0, 0)) {
		return store.Record{}, fmt.Errorf("birth_date must be YYYY-MM-DD and represent an age up to 130")
	}
	return record, nil
}

func validatePerson(person store.Person) error {
	person.Nickname = strings.TrimSpace(person.Nickname)
	person.Name = strings.TrimSpace(person.Name)
	person.BirthDate = strings.TrimSpace(person.BirthDate)
	if person.Nickname == "" || utf8.RuneCountInString(person.Nickname) > 32 {
		return fmt.Errorf("apelido is required and must have at most 32 characters")
	}
	if person.Name == "" || utf8.RuneCountInString(person.Name) > 100 {
		return fmt.Errorf("nome is required and must have at most 100 characters")
	}
	birthDate, err := time.Parse("2006-01-02", person.BirthDate)
	if err != nil || birthDate.After(time.Now().UTC()) {
		return fmt.Errorf("nascimento must use YYYY-MM-DD and cannot be in the future")
	}
	if len(person.Stack) > 20 {
		return fmt.Errorf("stack cannot contain more than 20 technologies")
	}
	for _, technology := range person.Stack {
		if technology == "" || utf8.RuneCountInString(technology) > 32 {
			return fmt.Errorf("each stack technology must have 1..32 characters")
		}
	}
	return nil
}

func normalizePhone(value string) string {
	value = strings.TrimSpace(value)
	var digits strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
		}
	}
	return digits.String()
}
