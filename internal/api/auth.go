package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Principal comes from trusted server context, independently of an employee card.
type Principal struct {
	Scope                 string
	Read, Manage, Command bool
	DeviceCode            *string
}
type Authenticator func(*http.Request) (Principal, error)

// StaticTokens is an explicit local development adapter, not enterprise IAM.
func StaticTokens(tokens map[string]Principal) Authenticator {
	copied := make(map[string]Principal, len(tokens))
	for token, p := range tokens {
		copied[token] = p
	}
	return func(r *http.Request) (Principal, error) {
		headers := r.Header.Values("Authorization")
		if len(headers) == 1 {
			parts := strings.Fields(headers[0])
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				for token, p := range copied {
					if token != "" && p.Scope != "" && subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) == 1 {
						return p, nil
					}
				}
			}
		}
		return Principal{}, fail(401, "UNAUTHENTICATED", "Требуется аутентификация")
	}
}
