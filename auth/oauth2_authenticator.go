package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// OAuth2Authenticator implements OAuth 2.0/OIDC authentication
type OAuth2Authenticator struct {
	verifier     TokenVerifier
	tokenCache   sync.Map // Thread-safe map for caching tokens
	clientID     string
	clientSecret string
}

// TokenVerifier defines an interface for verifying OAuth2/OIDC tokens
type TokenVerifier interface {
	// VerifyToken verifies a token and returns claims if valid
	VerifyToken(ctx context.Context, token string) (map[string]interface{}, error)
}

// OAuth2Option defines options for configuring OAuth2 authenticator
type OAuth2Option func(*OAuth2Authenticator)

// NewOAuth2Authenticator creates a new OAuth2 authenticator with a token verifier
func NewOAuth2Authenticator(verifier TokenVerifier, opts ...OAuth2Option) *OAuth2Authenticator {
	auth := &OAuth2Authenticator{
		verifier: verifier,
	}

	for _, opt := range opts {
		opt(auth)
	}

	return auth
}

// WithClientCredentials sets OAuth client credentials
func WithClientCredentials(clientID, clientSecret string) OAuth2Option {
	return func(a *OAuth2Authenticator) {
		a.clientID = clientID
		a.clientSecret = clientSecret
	}
}

// Authenticate validates OAuth2/OIDC tokens
func (a *OAuth2Authenticator) Authenticate(r *http.Request) bool {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" {
		return false
	}

	// Check cache first
	if _, found := a.tokenCache.Load(tokenStr); found {
		return true
	}

	// Verify the token
	claims, err := a.verifier.VerifyToken(r.Context(), tokenStr)
	if err != nil {
		return false
	}

	// Cache the valid token with expiry
	expiry := time.Now().Add(5 * time.Minute) // Default expiry
	if exp, ok := claims["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		if expTime.After(time.Now()) {
			expiry = expTime
		}
	}

	a.tokenCache.Store(tokenStr, expiry)
	return true
}

// GetAuthInfo returns OAuth token info
func (a *OAuth2Authenticator) GetAuthInfo(r *http.Request) map[string]interface{} {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return map[string]interface{}{"auth_type": "oauth2", "valid": false}
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" {
		return map[string]interface{}{"auth_type": "oauth2", "valid": false}
	}

	// Try to get from the verified tokens cache or re-validate
	claims, err := a.verifier.VerifyToken(r.Context(), tokenStr)
	if err != nil {
		return map[string]interface{}{"auth_type": "oauth2", "valid": false}
	}

	// Add auth_type to claims
	result := make(map[string]interface{})
	for k, v := range claims {
		result[k] = v
	}
	result["auth_type"] = "oauth2"

	return result
}

// DefaultOIDCVerifier implements a basic OIDC token verifier using well-known endpoints
type DefaultOIDCVerifier struct {
	issuerURL   string
	audience    string
	clientID    string
	jwksURI     string
	keyCache    sync.Map
	keyCacheTTL time.Duration
	lastRefresh time.Time
}

// NewDefaultOIDCVerifier creates a verifier that uses the OIDC provider's JWKS endpoint
func NewDefaultOIDCVerifier(issuerURL, clientID string) (*DefaultOIDCVerifier, error) {
	// In a real implementation, we would discover the JWKS URI from the provider's well-known
	// configuration endpoint and set up key rotation.
	// For simplicity, we'll assume the JWKS URI is <issuerURL>/.well-known/jwks.json
	return &DefaultOIDCVerifier{
		issuerURL:   issuerURL,
		clientID:    clientID,
		audience:    clientID,
		jwksURI:     issuerURL + "/.well-known/jwks.json",
		keyCacheTTL: 24 * time.Hour,
	}, nil
}

// VerifyToken validates an OIDC token and returns its claims
func (v *DefaultOIDCVerifier) VerifyToken(ctx context.Context, token string) (map[string]interface{}, error) {
	// Parse the token without verification first to get the header
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// We're just parsing to extract claims, not validating yet
		return nil, nil
	})

	// Check if token parsing failed for any reason other than signature verification
	if err != nil && !strings.Contains(err.Error(), "key is of invalid type") {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract the header claims to get the key ID (kid)
	kidValue, ok := parsedToken.Header["kid"]
	if !ok {
		return nil, errors.New("token missing key ID (kid) in header")
	}

	kid, ok := kidValue.(string)
	if !ok {
		return nil, errors.New("token key ID (kid) is not a string")
	}

	// Get the public key from JWKS
	publicKey, err := v.getPublicKeyFromJWKS(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Now verify the token with the correct key
	parsedToken, err = jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// Validate the algorithm
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if !parsedToken.Valid {
		return nil, errors.New("token is not valid")
	}

	// Extract claims
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims format")
	}

	// Validate issuer
	if iss, ok := claims["iss"].(string); !ok || iss != v.issuerURL {
		return nil, errors.New("invalid token issuer")
	}

	// Validate audience
	if aud, ok := claims["aud"].(string); ok {
		if aud != v.audience {
			return nil, errors.New("invalid token audience")
		}
	} else if audArray, ok := claims["aud"].([]interface{}); ok {
		found := false
		for _, a := range audArray {
			if audStr, ok := a.(string); ok && audStr == v.audience {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("audience not found in token")
		}
	} else {
		return nil, errors.New("invalid audience format in token")
	}

	// Validate expiration
	if exp, ok := claims["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		if time.Now().After(expTime) {
			return nil, errors.New("token has expired")
		}
	} else {
		return nil, errors.New("missing expiration time in token")
	}

	// Convert claims to map
	claimsMap := make(map[string]interface{})
	for key, val := range claims {
		claimsMap[key] = val
	}

	return claimsMap, nil
}

// getPublicKeyFromJWKS fetches and caches public keys from the JWKS endpoint
func (v *DefaultOIDCVerifier) getPublicKeyFromJWKS(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Check cache first
	if cachedKey, found := v.keyCache.Load(kid); found {
		keyInfo, ok := cachedKey.(keyInfo)
		if ok && time.Now().Before(keyInfo.expiry) {
			return keyInfo.publicKey, nil
		}
		v.keyCache.Delete(kid) // Remove expired key
	}

	// Check if we need to refresh the JWKS
	if time.Since(v.lastRefresh) > v.keyCacheTTL {
		if err := v.refreshJWKS(ctx); err != nil {
			return nil, err
		}
	}

	// Try to find the key in the newly refreshed cache
	if cachedKey, found := v.keyCache.Load(kid); found {
		if keyInfo, ok := cachedKey.(keyInfo); ok {
			return keyInfo.publicKey, nil
		}
	}

	return nil, fmt.Errorf("key with ID %s not found in JWKS", kid)
}

// keyInfo holds cached public key information
type keyInfo struct {
	publicKey *rsa.PublicKey
	expiry    time.Time
}

// refreshJWKS fetches the latest keys from the JWKS endpoint
func (v *DefaultOIDCVerifier) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", v.jwksURI, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get JWKS, status code: %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kid string   `json:"kid"`
			Kty string   `json:"kty"`
			Alg string   `json:"alg"`
			Use string   `json:"use"`
			N   string   `json:"n"`
			E   string   `json:"e"`
			X5c []string `json:"x5c"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("error parsing JWKS: %w", err)
	}

	// Store each key in the cache
	expiry := time.Now().Add(v.keyCacheTTL)
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue // Skip non-RSA keys
		}

		// Either use X5c (certificate) or n/e parameters to build the key
		var publicKey *rsa.PublicKey
		if len(key.X5c) > 0 {
			// Use the first certificate
			certData, err := base64.StdEncoding.DecodeString(key.X5c[0])
			if err != nil {
				continue // Skip this key if there's an error
			}

			cert, err := x509.ParseCertificate(certData)
			if err != nil {
				continue
			}

			rsaKey, ok := cert.PublicKey.(*rsa.PublicKey)
			if !ok {
				continue
			}
			publicKey = rsaKey
		} else {
			// Use modulus (n) and exponent (e)
			nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
			if err != nil {
				continue
			}

			eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
			if err != nil {
				continue
			}

			// Convert the exponent bytes to int
			var eInt int
			for i := 0; i < len(eBytes); i++ {
				eInt = eInt<<8 + int(eBytes[i])
			}

			publicKey = &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: eInt,
			}
		}

		if publicKey != nil {
			v.keyCache.Store(key.Kid, keyInfo{
				publicKey: publicKey,
				expiry:    expiry,
			})
		}
	}

	v.lastRefresh = time.Now()
	return nil
}
