package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func maxBodyBytes() int64 {
	const defaultMax = 1048576 // 1 MiB
	v := strings.TrimSpace(os.Getenv("HTTP_MAX_BODY_BYTES"))
	if v == "" {
		return defaultMax
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return defaultMax
	}
	return n
}

// decodeJSONBody lê o corpo com limite (HTTP_MAX_BODY_BYTES, default 1 MiB).
// Responde 413 / 400 e devolve false em caso de falha.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	maxB := maxBodyBytes()
	body := http.MaxBytesReader(w, r.Body, maxB)
	defer body.Close()

	dec := json.NewDecoder(body)
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload excede o tamanho máximo permitido")
			return false
		}
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "payload inválido: corpo vazio")
			return false
		}
		writeError(w, http.StatusBadRequest, "payload inválido")
		return false
	}
	return true
}
