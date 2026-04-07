package http

import (
	"net/http"
	"strings"
)

// simulationLLMAllowed é true quando pode chamar OpenAI (insight / enrich): utilizador
// com sub Clerk válido, ou modo AUTH_SKIP (dev).
func (s *Server) simulationLLMAllowed(r *http.Request) bool {
	if s.authDevSkip {
		return true
	}
	if s.clerkVerifier == nil {
		return false
	}
	sub, _, ok, err := s.clerkVerifier.OptionalUserClaims(r)
	if err != nil || !ok {
		return false
	}
	return strings.TrimSpace(sub) != ""
}
