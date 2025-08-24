package activitypub

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// KeyManager handles cryptographic keys for ActivityPub
type KeyManager struct {
	storage *Storage
	keyPair *KeyPair
}

// NewKeyManager creates a new key manager instance
func NewKeyManager(storage *Storage) *KeyManager {
	return &KeyManager{
		storage: storage,
	}
}

// InitializeKeys loads or generates RSA key pair for ActivityPub signatures
func (km *KeyManager) InitializeKeys(actorID string) error {
	// Try to load existing keys
	keys, err := km.storage.LoadKeys()
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	if keys != nil {
		km.keyPair = keys
		slog.Info("Loaded existing ActivityPub keys", "keyId", keys.KeyID)
		return nil
	}

	// Generate new key pair
	slog.Info("Generating new ActivityPub RSA key pair")
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key to PEM
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyPEMBytes := pem.EncodeToMemory(privateKeyPEM)

	// Encode public key to PEM
	publicKeyPKCS1, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyPKCS1,
	}
	publicKeyPEMBytes := pem.EncodeToMemory(publicKeyPEM)

	// Create key pair
	keyPair := &KeyPair{
		PrivateKeyPem: string(privateKeyPEMBytes),
		PublicKeyPem:  string(publicKeyPEMBytes),
		KeyID:         actorID + "#main-key",
		CreatedAt:     time.Now(),
	}

	// Save keys
	err = km.storage.SaveKeys(keyPair)
	if err != nil {
		return fmt.Errorf("failed to save keys: %w", err)
	}

	km.keyPair = keyPair
	slog.Info("Generated and saved new ActivityPub keys", "keyId", keyPair.KeyID)
	return nil
}

// GetPublicKeyPEM returns the public key in PEM format
func (km *KeyManager) GetPublicKeyPEM() string {
	if km.keyPair == nil {
		return ""
	}
	return km.keyPair.PublicKeyPem
}

// GetKeyID returns the key ID for HTTP signatures
func (km *KeyManager) GetKeyID() string {
	if km.keyPair == nil {
		return ""
	}
	return km.keyPair.KeyID
}

// SignRequest signs an HTTP request using HTTP signatures specification
func (km *KeyManager) SignRequest(req *http.Request, body []byte) error {
	if km.keyPair == nil {
		return fmt.Errorf("no key pair available for signing")
	}

	// Parse private key
	block, _ := pem.Decode([]byte(km.keyPair.PrivateKeyPem))
	if block == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Add required headers
	now := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", now)

	if req.Host == "" && req.URL != nil {
		req.Header.Set("Host", req.URL.Host)
	}

	// Add digest header if body is present
	if len(body) > 0 {
		hash := sha256.Sum256(body)
		digest := "SHA-256=" + base64.StdEncoding.EncodeToString(hash[:])
		req.Header.Set("Digest", digest)
	}

	// Create signature string
	signatureString, err := km.buildSignatureString(req, []string{"(request-target)", "host", "date", "digest"})
	if err != nil {
		return fmt.Errorf("failed to build signature string: %w", err)
	}

	// Sign the signature string
	hash := sha256.Sum256([]byte(signatureString))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	// Create signature header
	signatureB64 := base64.StdEncoding.EncodeToString(signature)
	signatureHeader := fmt.Sprintf(`keyId="%s",algorithm="rsa-sha256",headers="(request-target) host date digest",signature="%s"`,
		km.keyPair.KeyID, signatureB64)

	req.Header.Set("Signature", signatureHeader)
	return nil
}

// buildSignatureString creates the string to be signed according to HTTP signatures spec
func (km *KeyManager) buildSignatureString(req *http.Request, headers []string) (string, error) {
	var parts []string

	for _, header := range headers {
		var value string

		switch strings.ToLower(header) {
		case "(request-target)":
			// Special pseudo-header for request target
			target := strings.ToLower(req.Method) + " " + req.URL.RequestURI()
			value = target
		case "host":
			value = req.Header.Get("Host")
			if value == "" && req.URL != nil {
				value = req.URL.Host
			}
		default:
			value = req.Header.Get(header)
		}

		if value == "" && header != "digest" {
			return "", fmt.Errorf("header %s not found or empty", header)
		}

		parts = append(parts, strings.ToLower(header)+": "+value)
	}

	return strings.Join(parts, "\n"), nil
}

// VerifySignature verifies an HTTP signature from an incoming request
func (km *KeyManager) VerifySignature(req *http.Request, body []byte, publicKeyPEM string) error {
	signatureHeader := req.Header.Get("Signature")
	if signatureHeader == "" {
		return fmt.Errorf("no signature header found")
	}

	// Parse signature header
	params, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return fmt.Errorf("failed to parse signature header: %w", err)
	}

	keyId, exists := params["keyId"]
	if !exists {
		return fmt.Errorf("keyId not found in signature header")
	}

	algorithm, exists := params["algorithm"]
	if !exists {
		return fmt.Errorf("algorithm not found in signature header")
	}

	if algorithm != "rsa-sha256" {
		return fmt.Errorf("unsupported signature algorithm: %s", algorithm)
	}

	headersParam, exists := params["headers"]
	if !exists {
		return fmt.Errorf("headers not found in signature header")
	}

	signatureB64, exists := params["signature"]
	if !exists {
		return fmt.Errorf("signature not found in signature header")
	}

	// Parse headers to verify
	headers := strings.Fields(headersParam)

	// Verify digest if present
	if containsString(headers, "digest") {
		expectedDigest := req.Header.Get("Digest")
		if expectedDigest == "" {
			return fmt.Errorf("digest header missing but required for verification")
		}

		if !strings.HasPrefix(expectedDigest, "SHA-256=") {
			return fmt.Errorf("unsupported digest algorithm")
		}

		expectedHashB64 := strings.TrimPrefix(expectedDigest, "SHA-256=")
		actualHash := sha256.Sum256(body)
		actualHashB64 := base64.StdEncoding.EncodeToString(actualHash[:])

		if expectedHashB64 != actualHashB64 {
			return fmt.Errorf("digest verification failed")
		}
	}

	// Build signature string
	signatureString, err := km.buildSignatureString(req, headers)
	if err != nil {
		return fmt.Errorf("failed to build signature string for verification: %w", err)
	}

	// Parse public key
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}

	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Verify signature
	hash := sha256.Sum256([]byte(signatureString))
	err = rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	slog.Info("HTTP signature verification successful", "keyId", keyId)
	return nil
}

// parseSignatureHeader parses the Signature header into key-value pairs
func parseSignatureHeader(header string) (map[string]string, error) {
	params := make(map[string]string)

	// Split by commas, but be careful with quoted values
	var parts []string
	var current string
	inQuotes := false

	for i, char := range header {
		if char == '"' {
			inQuotes = !inQuotes
		}

		if char == ',' && !inQuotes {
			parts = append(parts, current)
			current = ""
			continue
		}

		current += string(char)

		if i == len(header)-1 {
			parts = append(parts, current)
		}
	}

	// Parse each part
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		eqIndex := strings.Index(part, "=")
		if eqIndex == -1 {
			return nil, fmt.Errorf("invalid signature parameter: %s", part)
		}

		key := strings.TrimSpace(part[:eqIndex])
		value := strings.TrimSpace(part[eqIndex+1:])

		// Remove quotes if present
		if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			value = value[1 : len(value)-1]
		}

		params[key] = value
	}

	return params, nil
}

// containsString checks if a slice contains a string
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// FetchPublicKey fetches a public key from a remote actor
func (km *KeyManager) FetchPublicKey(keyID string) (string, error) {
	// Parse key ID to get actor URL
	keyURL, err := url.Parse(keyID)
	if err != nil {
		return "", fmt.Errorf("invalid key ID URL: %w", err)
	}

	// Remove fragment to get actor URL
	actorURL := keyURL.Scheme + "://" + keyURL.Host + keyURL.Path
	if keyURL.Fragment != "" {
		// Remove fragment part for actor URL
		actorURL = strings.TrimSuffix(actorURL, "#"+keyURL.Fragment)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create request for actor
	req, err := http.NewRequest("GET", actorURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Sn/1.0")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch actor, status: %d", resp.StatusCode)
	}

	// Parse actor response
	var actor Actor
	err = parseJSONResponse(resp, &actor)
	if err != nil {
		return "", fmt.Errorf("failed to parse actor response: %w", err)
	}

	// Extract public key
	if actor.PublicKey == nil {
		return "", fmt.Errorf("no public key found in actor")
	}

	if actor.PublicKey.ID != keyID {
		return "", fmt.Errorf("key ID mismatch: expected %s, got %s", keyID, actor.PublicKey.ID)
	}

	return actor.PublicKey.PublicKeyPem, nil
}

// GenerateActivityID generates a unique ID for an activity
func GenerateActivityID(baseURL, actorUsername string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s/@%s/activities/%d", baseURL, actorUsername, timestamp)
}

// GenerateObjectID generates a unique ID for an object
func GenerateObjectID(baseURL, actorUsername string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s/@%s/objects/%d", baseURL, actorUsername, timestamp)
}

// NormalizeHeaders normalizes header names for consistent signature verification
func NormalizeHeaders(headers []string) []string {
	normalized := make([]string, len(headers))
	for i, header := range headers {
		normalized[i] = strings.ToLower(strings.TrimSpace(header))
	}
	sort.Strings(normalized)
	return normalized
}
