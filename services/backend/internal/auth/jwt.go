package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ctxKey string

const UserIDKey ctxKey = "userID"

type Claims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}

func IssueAccess(secret string, userID uuid.UUID, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

func ParseAccess(secret, token string) (uuid.UUID, error) {
	c, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	cl, ok := c.Claims.(*Claims)
	if !ok || !c.Valid {
		return uuid.Nil, errors.New("invalid token")
	}
	return uuid.Parse(cl.UserID)
}

func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return h[len(p):]
	}
	return ""
}

func UserFromRequest(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(UserIDKey)
	if v == nil {
		return uuid.Nil, false
	}
	u, ok := v.(uuid.UUID)
	return u, ok
}

func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, UserIDKey, id)
}
