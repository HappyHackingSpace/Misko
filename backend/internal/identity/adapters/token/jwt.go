// Package token issues and verifies HS256 JWTs for identity use cases.
package token

import (
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	"github.com/golang-jwt/jwt/v5"
	"slices"
	"time"
)

const (
	MinSecretBytes = 32
	maxTokenBytes  = 4096
)

var ErrInvalidToken = errors.New("invalid token")

type JWT struct {
	secret           []byte
	issuer, audience string
	ttl              time.Duration
	now              func() time.Time
	parser           *jwt.Parser
}

type claims struct {
	jwt.RegisteredClaims
	SessionVersion int `json:"sv"`
}

func NewJWT(secret []byte, issuer, audience string, ttl time.Duration, now func() time.Time) (*JWT, error) {
	if len(secret) < MinSecretBytes || issuer == "" || audience == "" || ttl <= 0 || now == nil {
		return nil, errors.New("JWT requires a secret of at least 32 bytes, issuer, audience, positive TTL and clock")
	}
	return &JWT{
		secret: slices.Clone(secret), issuer: issuer, audience: audience, ttl: ttl, now: now,
		// Only HS256 is accepted; the header cannot select another algorithm.
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithTimeFunc(now),
		),
	}, nil
}

func (j *JWT) Issue(c application.Claims) (application.Token, error) {
	if c.UserID == "" || c.SessionVersion < 1 {
		return application.Token{}, errors.New("token claims require a user and session version")
	}
	// NumericDate has second precision; truncate so ExpiresAt matches the claim.
	issuedAt := j.now().UTC().Truncate(time.Second)
	expiresAt := issuedAt.Add(j.ttl)
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Subject:   c.UserID,
			Audience:  jwt.ClaimStrings{j.audience},
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		SessionVersion: c.SessionVersion,
	}).SignedString(j.secret)
	if err != nil {
		return application.Token{}, fmt.Errorf("sign token: %w", err)
	}
	return application.Token{Value: raw, ExpiresAt: expiresAt}, nil
}

func (j *JWT) Verify(raw string) (application.Claims, error) {
	if raw == "" || len(raw) > maxTokenBytes {
		return application.Claims{}, ErrInvalidToken
	}
	var parsed claims
	token, err := j.parser.ParseWithClaims(raw, &parsed, func(*jwt.Token) (any, error) { return j.secret, nil })
	if err != nil || !token.Valid || parsed.Subject == "" || parsed.SessionVersion < 1 {
		return application.Claims{}, ErrInvalidToken
	}
	return application.Claims{UserID: parsed.Subject, SessionVersion: parsed.SessionVersion}, nil
}
