package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type JWTClaims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

type JWTMiddleware struct {
	secret []byte
	expire time.Duration
}

func NewJWTMiddleware(secret string, expire time.Duration) *JWTMiddleware {
	return &JWTMiddleware{
		secret: []byte(secret),
		expire: expire,
	}
}

// GenerateToken creates a new JWT for a user
func (m *JWTMiddleware) GenerateToken(username string) (string, error) {
	claims := &JWTClaims{
		Sub: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// GenerateTokenByID creates a JWT using user ID as sub
func (m *JWTMiddleware) GenerateTokenByID(userID int64) (string, error) {
	claims := &JWTClaims{
		Sub: strconv.FormatInt(userID, 10),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ValidateToken parses and validates a JWT, returns claims or error
func (m *JWTMiddleware) ValidateToken(tokenStr string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

// AuthSkipper - paths that don't need auth
func AuthSkipper(c echo.Context) bool {
	path := c.Path()
	// Public paths
	publicPrefixes := []string{
		"/api/auth/login",
		"/api/auth/register",
		"/api/auth/api_keys", // depends on API key
		"/api/tracks",         // list/search is public
		"/api/albums",
		"/api/artists",
		"/api/playlists",
		"/api/search",
		"/api/stats",
		"/api/system",
		"/api/covers",
		"/stream/",
		"/",
	}
	for _, p := range publicPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func int64ToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	signed := n < 0
	if signed {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if signed {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}


