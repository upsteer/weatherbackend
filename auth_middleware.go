package main

import "net/http"

const apiSecretHeader = "X-API-Secret"

func requireAPISecret(next http.HandlerFunc, apiSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiSecretHeader) != apiSecret {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}
