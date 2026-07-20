// Copyright (C) 2025 Thinline Dynamic Solutions
//
// relay_account_api.go — admin HTTP handlers for relay account sign-in/migration
// and the Nominatim subscription they gate. Thin wrappers around relay_account.go;
// kept server-side (never call the relay directly from the browser here) so the
// operator's password only ever crosses this server's own admin session, not an
// extra client-side hop, and so the persisted refresh token stays out of the SPA.

package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// RelayAccountStatusHandler reports sign-in + Nominatim subscription state.
func (admin *Admin) RelayAccountStatusHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(admin.Controller.GetRelayAccountStatus())
}

// RelayAccountLoginHandler signs in with a username/password already linked
// to this server's relay API key.
func (admin *Admin) RelayAccountLoginHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := admin.Controller.RelayAccountLogin(body.Username, body.Password); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(admin.Controller.GetRelayAccountStatus())
}

// RelayAccountCreateHandler creates a portal account for this server's
// configured relay API key (Thinline Radio Services) and signs in.
func (admin *Admin) RelayAccountCreateHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Username string `json:"username"` // legacy alias
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" {
		email = strings.TrimSpace(body.Username)
	}
	if err := admin.Controller.RelayAccountCreate(email, body.Password); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(admin.Controller.GetRelayAccountStatus())
}

// RelayAccountMigrateHandler completes a one-time migration invite (from the
// emailed link, token pasted in by the operator) — creates the account,
// links it to this server's existing API key, and signs in.
func (admin *Admin) RelayAccountMigrateHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := admin.Controller.RelayAccountMigrate(body.Token, body.Username, body.Password); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(admin.Controller.GetRelayAccountStatus())
}

// RelayAccountRequestMigrationHandler (re)sends a migration invite email for
// this server's existing API key — the in-admin fallback if the original
// proactive campaign email was missed, deleted, or expired.
func (admin *Admin) RelayAccountRequestMigrationHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := admin.Controller.RelayAccountRequestMigrationInvite(); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// RelayAccountLogoutHandler signs out (revokes the session on the relay).
func (admin *Admin) RelayAccountLogoutHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := admin.Controller.RelayAccountLogout(); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// NominatimSubscribeHandler starts a Stripe Checkout session for the
// Nominatim geocoding add-on and returns the hosted checkout URL for the
// browser to open in a new tab. Keeps the Stripe secret key entirely on
// relay's side — this server just forwards the API key and relays back
// whatever URL comes back.
func (admin *Admin) NominatimSubscribeHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	checkoutURL, err := admin.Controller.NominatimSubscribe()
	if err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"checkout_url": checkoutURL})
}

// RelayBillingCatalogHandler returns active relay billing plans and this
// server's entitlements (pulled from relay using the configured API key).
func (admin *Admin) RelayBillingCatalogHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	catalog, err := admin.Controller.RelayBillingCatalog()
	if err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(catalog)
}

// RelayPlanSubscribeHandler starts Stripe Checkout for a billing plan slug.
func (admin *Admin) RelayPlanSubscribeHandler(w http.ResponseWriter, r *http.Request) {
	t := admin.GetAuthorization(r)
	if !admin.ValidateToken(t) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PlanSlug string `json:"plan_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	checkoutURL, err := admin.Controller.RelayPlanSubscribe(body.PlanSlug)
	if err != nil {
		respondRelayAccountError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"checkout_url": checkoutURL,
		"plan_slug":    strings.TrimSpace(strings.ToLower(body.PlanSlug)),
	})
}

func respondRelayAccountError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": message})
}
