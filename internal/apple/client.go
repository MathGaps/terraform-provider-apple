// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package apple

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/golang-jwt/jwt/v5"
)

const HostURL string = "https://api.appstoreconnect.apple.com"

type Client struct {
	HostURL    string
	HTTPClient *http.Client
	Token      string
	IssuerID   string
	KeyID      string
}

// NewClient creates a new Apple API client with a generated JWT token.
//
// The token is minted once here and is immutable for the life of the client.
// App Store Connect tokens are valid for 20 minutes, which comfortably outlives
// a single Terraform command: every operation this provider performs is one
// fast REST call, and `plan` and `apply` run as separate processes that each
// construct their own client. The signing key is therefore not retained.
func NewClient(issuerID, keyID, privateKeyPEM string, scope []string) (*Client, error) {
	token, err := createToken(keyID, issuerID, privateKeyPEM, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %v", err)
	}

	return &Client{
		HostURL:    HostURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Token:      token,
		IssuerID:   issuerID,
		KeyID:      keyID,
	}, nil
}

func createToken(keyID, issuerID, privateKeyPEM string, scope []string) (string, error) {
	audience := "appstoreconnect-v1"

	// Token expiration (20 min from now)
	now := time.Now()
	issuedAt := now.Unix()
	expiresAt := now.Add(20 * time.Minute).Unix()

	// Decode the private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil || block.Type != "PRIVATE KEY" {
		return "", fmt.Errorf("failed to decode PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %v", err)
	}

	// Create the JWT claims
	claims := jwt.MapClaims{
		"iss": issuerID,
		"iat": issuedAt,
		"exp": expiresAt,
		"aud": audience,
	}

	// The scope claim is omitted unless it actually restricts something. App
	// Store Connect rejects a token carrying an empty or null scope with a 400
	// ENTITY_INVALID titled "JSON processing failed" — an error about the token
	// payload that reads like a complaint about the request body. Since the
	// provider defaults to no scope, sending the claim unconditionally broke
	// every API call.
	if len(scope) > 0 {
		claims["scope"] = scope
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)

	// Set the "kid" in the header
	token.Header["kid"] = keyID

	// Sign the token with the private key
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %v", err)
	}

	return signedToken, nil
}

func (c *Client) doRequest(req *http.Request, authToken *string) ([]byte, error) {
	return c.doRequestAccepting(req, authToken, "application/json")
}

// doRequestAccepting is doRequest for the few endpoints whose success body is
// not JSON. GET /v1/subscriptionOfferCodeOneTimeUseCodes/{id}/values answers
// text/csv -- the generated codes as a file -- so asking it for JSON alone
// would be asking for a representation it does not have. Errors are JSON
// either way, which is why the error handling below is shared.
func (c *Client) doRequestAccepting(req *http.Request, authToken *string, accept string) ([]byte, error) {
	token := c.Token

	if authToken != nil {
		token = *authToken
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// Handle different success status codes
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return body, nil
	}

	// Prefer Apple's own structured error message when it supplies one.
	var errorResponse models.ErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err == nil && len(errorResponse.Errors) > 0 {
		return nil, fmt.Errorf("API error (status %d): %s - %s",
			res.StatusCode, errorResponse.Errors[0].Title, errorResponse.Errors[0].Detail)
	}

	// A 401 means the credentials themselves were rejected. Re-signing a token
	// with the same key would produce an equivalent token and be rejected
	// identically, so report the failure instead of retrying.
	if res.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (status 401): App Store Connect rejected the API credentials. "+
			"Verify the issuer ID, key ID, and private key are correct and that the key has not been revoked in "+
			"App Store Connect under Users and Access > Keys. Response: %s", body)
	}

	return nil, fmt.Errorf("HTTP error: status %d, body: %s", res.StatusCode, body)
}
