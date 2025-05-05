package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// JWTAuthenticator implements JWT-based authentication
type JWTAuthenticator struct {
	secretKey    interface{} // Can be []byte for HMAC or *rsa.PublicKey for RSA
	issuer       string
	audience     string
	allowedAlgs  []string
	claimsParser func(claims jwt.MapClaims) (bool, error)
	expiryWindow time.Duration
	tokenCache   map[string]time.Time // Simple token cache for performance
}

// JWTAuthOption defines options for configuring the JWT authenticator
type JWTAuthOption func(*JWTAuthenticator)

// NewJWTAuthenticator creates a new JWT authenticator with the given options
func NewJWTAuthenticator(secretKey interface{}, opts ...JWTAuthOption) *JWTAuthenticator {
	auth := &JWTAuthenticator{
		secretKey:    secretKey,
		allowedAlgs:  []string{"HS256", "HS384", "HS512"},
		expiryWindow: 0, // No leeway by default
		tokenCache:   make(map[string]time.Time),
	}

	// Apply options
	for _, opt := range opts {
		opt(auth)
	}

	return auth
}

// WithIssuer sets the required issuer for JWT tokens
func WithIssuer(issuer string) JWTAuthOption {
	return func(a *JWTAuthenticator) {
		a.issuer = issuer
	}
}

// WithAudience sets the required audience for JWT tokens
func WithAudience(audience string) JWTAuthOption {
	return func(a *JWTAuthenticator) {
		a.audience = audience
	}
}

// WithRSAPublicKey configures the authenticator to use RSA public key for verification
func WithRSAPublicKey(publicKey *rsa.PublicKey) JWTAuthOption {
	return func(a *JWTAuthenticator) {
		a.secretKey = publicKey
		a.allowedAlgs = []string{"RS256", "RS384", "RS512"}
	}
}

// WithAllowedAlgorithms sets the allowed signing algorithms
func WithAllowedAlgorithms(algs []string) JWTAuthOption {
	return func(a *JWTAuthenticator) {
		if len(algs) > 0 {
			a.allowedAlgs = algs
		}
	}
}

// WithExpiryWindow adds a leeway window for token expiration checking
func WithExpiryWindow(window time.Duration) JWTAuthOption {
	return func(a *JWTAuthenticator) {
		a.expiryWindow = window
	}
}

// WithClaimsValidator adds custom claims validation
func WithClaimsValidator(validator func(claims jwt.MapClaims) (bool, error)) JWTAuthOption {
	return func(a *JWTAuthenticator) {
		a.claimsParser = validator
	}
}

// Authenticate validates JWT tokens from the Authorization header
func (a *JWTAuthenticator) Authenticate(r *http.Request) bool {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" {
		return false
	}

	// Check cache first for performance
	if expiry, found := a.tokenCache[tokenStr]; found {
		if time.Now().Before(expiry) {
			return true
		}
		// Token expired, remove from cache
		delete(a.tokenCache, tokenStr)
	}

	// Parse the token
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Validate the algorithm
		alg := token.Method.Alg()
		valid := false
		for _, allowedAlg := range a.allowedAlgs {
			if alg == allowedAlg {
				valid = true
				break
			}
		}

		if !valid {
			return nil, fmt.Errorf("unexpected signing method: %v", alg)
		}

		return a.secretKey, nil
	})

	if err != nil || !token.Valid {
		return false
	}

	// Validate claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}

	// Check expiration with window
	if exp, ok := claims["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		if time.Now().Add(a.expiryWindow).After(expTime) {
			return false
		}
	}

	// Check issuer
	if a.issuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != a.issuer {
			return false
		}
	}

	// Check audience
	if a.audience != "" {
		if aud, ok := claims["aud"].(string); !ok || aud != a.audience {
			// Check for audience array
			if audArray, ok := claims["aud"].([]interface{}); ok {
				found := false
				for _, ae := range audArray {
					if audStr, ok := ae.(string); ok && audStr == a.audience {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			} else {
				return false
			}
		}
	}

	// Apply custom claims validator if provided
	if a.claimsParser != nil {
		valid, _ := a.claimsParser(claims)
		if !valid {
			return false
		}
	}

	// Add to cache with expiry
	if exp, ok := claims["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		a.tokenCache[tokenStr] = expTime
	} else {
		// If no expiry, cache for a short time
		a.tokenCache[tokenStr] = time.Now().Add(5 * time.Minute)
	}

	return true
}

// GetAuthInfo returns the JWT claims
func (a *JWTAuthenticator) GetAuthInfo(r *http.Request) map[string]interface{} {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return map[string]interface{}{"auth_type": "jwt", "valid": false}
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse the token without validation (we already validated in Authenticate)
	token, _ := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return nil, errors.New("no validation needed")
	})

	result := map[string]interface{}{
		"auth_type": "jwt",
	}

	if token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			for key, val := range claims {
				result[key] = val
			}
		}
	}

	return result
}
