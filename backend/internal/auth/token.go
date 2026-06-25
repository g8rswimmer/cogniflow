package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload for cogniflow session tokens.
type Claims struct {
	UserID      string   `json:"sub"`
	OrgID       string   `json:"org_id"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	OrgName     string   `json:"org_name"`
	Permissions []string `json:"permissions,omitempty"`
	jwt.RegisteredClaims
}

// Sign creates and signs a JWT with the given claims, secret, and TTL.
func Sign(claims Claims, secret []byte, ttl time.Duration) (string, error) {
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

// Verify parses and validates a JWT, returning the claims on success.
func Verify(tokenStr string, secret []byte) (Claims, error) {
	var claims Claims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("auth: invalid token: %w", err)
	}
	if !token.Valid {
		return Claims{}, fmt.Errorf("auth: token not valid")
	}
	return claims, nil
}
