// Package i18n fournit les textes de Rodage en français et en anglais.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang est une langue prise en charge.
type Lang string

const (
	FR Lang = "fr"
	EN Lang = "en"
)

// Parse convertit "fr", "fr_FR.UTF-8", "en-US"… en Lang (défaut : anglais).
func Parse(s string) Lang {
	s = strings.ToLower(s)
	if strings.HasPrefix(s, "fr") {
		return FR
	}
	return EN
}

// Detect devine la langue du système.
func Detect() Lang {
	for _, v := range []string{"RODAGE_LANG", "LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		if s := os.Getenv(v); s != "" && s != "C" && s != "POSIX" {
			return Parse(s)
		}
	}
	return detectOS()
}

// T traduit une clé, avec des arguments à la manière de fmt.Sprintf.
func T(l Lang, key string, args ...any) string {
	m, ok := catalog[key]
	if !ok {
		if len(args) > 0 {
			return key + fmt.Sprint(args...)
		}
		return key
	}
	s := m[0]
	if l == EN {
		s = m[1]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Has indique si une clé existe.
func Has(key string) bool { _, ok := catalog[key]; return ok }
