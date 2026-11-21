package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// The smallest JWT flow that a real client and the Plux gateway accept: RS256 access tokens signed with a key made at
// start-up, its public half at /auth/.well-known/jwks.json, opaque refresh tokens kept in memory.

const (
	issuer   = "https://api.apply-register.example/auth"
	audience = "apply-register"
	keyID    = "ar-1"
	ttl      = 15 * time.Minute
)

type signer struct{ key *rsa.PrivateKey }

func newSigner() *signer {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return &signer{key: k}
}

var b64 = base64.RawURLEncoding

func (s *signer) sign(claims map[string]any) string {
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": keyID})
	body, _ := json.Marshal(claims)
	unsigned := b64.EncodeToString(head) + "." + b64.EncodeToString(body)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return unsigned + "." + b64.EncodeToString(sig)
}

func (s *signer) verify(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("malformed signature")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&s.key.PublicKey, crypto.SHA256, sum[:], sig) != nil {
		return nil, errors.New("bad signature")
	}
	raw, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("malformed claims")
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return nil, errors.New("malformed claims")
	}
	if exp, ok := claims["exp"].(float64); !ok || time.Now().Unix() > int64(exp) {
		return nil, errors.New("token expired")
	}
	return claims, nil
}

func (s *signer) jwks() map[string]any {
	pub := s.key.PublicKey
	return map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": keyID,
		"n": b64.EncodeToString(pub.N.Bytes()), "e": b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}

func (a *api) issue(u *User) map[string]any {
	now := time.Now()
	access := a.jwt.sign(map[string]any{
		"iss": issuer, "aud": audience, "sub": u.ID, "role": u.Role, "org": u.OrgSlug, "region": u.Region,
		"segment": u.Segment, "iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	})
	refresh := randomID(24)
	a.db.refresh[refresh] = u.ID
	return map[string]any{"accessToken": access, "refreshToken": refresh, "expiresIn": int(ttl.Seconds()), "tokenType": "Bearer"}
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b64.EncodeToString(b)
}

// user is the signed-in caller, or nil after the response said why not.
func (a *api) user(w http.ResponseWriter, r *http.Request) *User {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		fail(w, http.StatusUnauthorized, "unauthenticated", "sign in first")
		return nil
	}
	claims, err := a.jwt.verify(strings.TrimPrefix(h, "Bearer "))
	if err != nil {
		fail(w, http.StatusUnauthorized, "unauthenticated", err.Error())
		return nil
	}
	sub, _ := claims["sub"].(string)
	a.db.mu.Lock()
	u := a.db.users[sub]
	a.db.mu.Unlock()
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthenticated", "the account is gone")
		return nil
	}
	return u
}

// otpStart accepts any phone or e-mail and "sends" a code: the test number +15555550100 and any address take 000000.
func (a *api) otpStart(w http.ResponseWriter, r *http.Request) {
	var in struct{ Phone, Email string }
	if !decode(w, r, &in) {
		return
	}
	who := strings.TrimSpace(in.Phone + in.Email)
	if who == "" {
		fail(w, http.StatusBadRequest, "invalid", "a phone or an email is needed")
		return
	}
	a.db.mu.Lock()
	challenge := randomID(12)
	a.db.otps[challenge] = who
	a.db.mu.Unlock()
	reply(w, http.StatusOK, map[string]any{"challenge": challenge, "sentTo": who, "expiresIn": 300, "devCode": "000000"})
}

func (a *api) otpVerify(w http.ResponseWriter, r *http.Request) {
	var in struct{ Challenge, Code string }
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	who, ok := a.db.otps[in.Challenge]
	if !ok || in.Code != "000000" {
		fail(w, http.StatusUnauthorized, "invalid_code", "the code is wrong or expired")
		return
	}
	delete(a.db.otps, in.Challenge)
	var u *User
	for _, x := range a.db.users {
		if x.Phone == who || strings.EqualFold(x.Email, who) {
			u = x
		}
	}
	if u == nil {
		u = &User{ID: a.db.next("usr"), Name: "New member", Role: "member", Region: "eu", Segment: "general"}
		if strings.Contains(who, "@") {
			u.Email = who
		} else {
			u.Phone = who
		}
		a.db.users[u.ID] = u
	}
	reply(w, http.StatusOK, a.issue(u))
}

func (a *api) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct{ RefreshToken string }
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	id, ok := a.db.refresh[in.RefreshToken]
	if !ok {
		fail(w, http.StatusUnauthorized, "invalid_refresh", "sign in again")
		return
	}
	delete(a.db.refresh, in.RefreshToken) // rotated: a refresh token works once
	reply(w, http.StatusOK, a.issue(a.db.users[id]))
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	var in struct{ RefreshToken string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	a.db.mu.Lock()
	delete(a.db.refresh, in.RefreshToken)
	a.db.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) deleteAccount(w http.ResponseWriter, r *http.Request) {
	u := a.user(w, r)
	if u == nil {
		return
	}
	a.db.mu.Lock()
	delete(a.db.users, u.ID)
	for t, id := range a.db.refresh {
		if id == u.ID {
			delete(a.db.refresh, t)
		}
	}
	a.db.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
