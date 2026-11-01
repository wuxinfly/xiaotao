package qoder

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SignOptions controls optional COSY request attributes.
type SignOptions struct {
	ClientType  string
	ModelKey    string
	ModelSource string
	Encode      *bool
	Now         func() time.Time
	NewKey      func() ([]byte, error)
	NewUUID     func() string
}

// SignedRequest holds the encoded body and headers for one COSY call.
type SignedRequest struct {
	Body    string
	Headers map[string]string
}

var b64Map [128]byte
var b64Unmap map[byte]byte

func init() {
	for i := range b64Map {
		b64Map[i] = 0
	}
	for i := 0; i < 64; i++ {
		b64Map[StdB64Alphabet[i]] = CustomB64Alphabet[i]
	}
	b64Map['='] = '$'
	b64Unmap = make(map[byte]byte, 65)
	for i := 0; i < 64; i++ {
		b64Unmap[CustomB64Alphabet[i]] = StdB64Alphabet[i]
	}
	b64Unmap['$'] = '='
}

// EncodeBody applies Qoder's custom Base64 substitution and three-segment rearrange.
func EncodeBody(plaintext string) string {
	std := base64.StdEncoding.EncodeToString([]byte(plaintext))
	var b strings.Builder
	b.Grow(len(std))
	for i := 0; i < len(std); i++ {
		b.WriteByte(b64Map[std[i]])
	}
	return rearrange(b.String())
}

// DecodeBody reverses EncodeBody; it is mainly for tests and diagnostics.
func DecodeBody(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	n := len(encoded)
	a := n / 3
	p3 := encoded[:a]
	p2 := encoded[a : n-a]
	p1 := encoded[n-a:]
	mapped := p1 + p2 + p3
	std := make([]byte, len(mapped))
	for i := 0; i < len(mapped); i++ {
		c, ok := b64Unmap[mapped[i]]
		if !ok {
			return "", fmt.Errorf("invalid COSY body character %q", mapped[i])
		}
		std[i] = c
	}
	raw, err := base64.StdEncoding.DecodeString(string(std))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func rearrange(s string) string {
	n := len(s)
	a := n / 3
	if a == 0 {
		return s
	}
	return s[n-a:] + s[a:n-a] + s[:a]
}

// SignaturePath removes the /algo prefix and query string.
func SignaturePath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	p := u.Path
	if strings.HasPrefix(p, "/algo") {
		p = p[len("/algo"):]
	}
	return p
}

func parsePublicKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(COSYRSAPublicKeyPEM))
	if block == nil {
		return nil, errors.New("invalid COSY RSA public key PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("COSY public key is not RSA")
	}
	return pub, nil
}

var cosyPublicKey = func() *rsa.PublicKey {
	key, err := parsePublicKey()
	if err != nil {
		panic(err)
	}
	return key
}()

func aesCBCEncrypt(key, iv, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := make([]byte, len(plaintext)+pad)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(pad)
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

func buildIdentity(uid, token, name, email string, newKey func() ([]byte, error)) (infoB64 string, key []byte, err error) {
	if newKey != nil {
		key, err = newKey()
	} else {
		rawKey := make([]byte, 8)
		_, err = rand.Read(rawKey)
		key = []byte(hex.EncodeToString(rawKey))
	}
	if err != nil {
		return "", nil, err
	}
	identity := map[string]any{
		"uid":                  uid,
		"security_oauth_token": token,
		"name":                 name,
		"aid":                  "",
		"email":                email,
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", nil, err
	}
	encrypted, err := aesCBCEncrypt(key, key, raw)
	if err != nil {
		return "", nil, err
	}
	return base64.StdEncoding.EncodeToString(encrypted), key, nil
}

func buildPayload(infoB64 string, now func() time.Time, newUUID func() string) string {
	requestID := newUUID
	if requestID == nil {
		requestID = newUUIDString
	}
	payload, _ := json.Marshal(map[string]any{
		"version":     "v1",
		"requestId":   requestID(),
		"info":        infoB64,
		"cosyVersion": COSYVersion,
		"ideVersion":  "",
	})
	return string(payload)
}

func newUUIDString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Sign produces a COSY signed request. body is the plaintext JSON; the returned
// Body is the wire format and must be sent verbatim.
func Sign(rawURL, body, uid, token, machineID string, opts SignOptions) (SignedRequest, error) {
	encode := true
	if opts.Encode != nil {
		encode = *opts.Encode
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	infoB64, aesKey, err := buildIdentity(uid, token, "", "", opts.NewKey)
	if err != nil {
		return SignedRequest{}, err
	}
	encryptedKey, err := rsa.EncryptPKCS1v15(rand.Reader, cosyPublicKey, aesKey)
	if err != nil {
		return SignedRequest{}, err
	}
	cosyKey := base64.StdEncoding.EncodeToString(encryptedKey)
	payload := buildPayload(infoB64, now, opts.NewUUID)
	payloadB64 := base64.StdEncoding.EncodeToString([]byte(payload))
	ts := fmt.Sprintf("%d", now().Unix())
	encodedBody := ""
	if encode {
		encodedBody = EncodeBody(body)
	}
	sum := md5.Sum([]byte(strings.Join([]string{payloadB64, cosyKey, ts, encodedBody, SignaturePath(rawURL)}, "\n")))
	sig := hex.EncodeToString(sum[:])
	clientType := opts.ClientType
	if clientType == "" {
		clientType = "5"
	}
	modelSource := opts.ModelSource
	if opts.ModelKey != "" && modelSource == "" {
		modelSource = "system"
	}
	headers := map[string]string{
		"Authorization":         "Bearer COSY." + payloadB64 + "." + sig,
		"Content-Type":          "application/json",
		"Cosy-Version":          COSYVersion,
		"Cosy-ClientType":       clientType,
		"Cosy-Data-Policy":      "agree",
		"Cosy-Date":             ts,
		"Cosy-Key":              cosyKey,
		"Cosy-MachineId":        machineID,
		"Cosy-MachineToken":     machineID,
		"Cosy-MachineType":      clientType,
		"Cosy-Scene":            "assistant",
		"Cosy-User":             uid,
		"Cosy-Business-Product": "cli",
		"Cosy-Business-Type":    "agent",
		"Login-Version":         "v2",
	}
	if opts.ModelKey != "" {
		headers["X-Model-Key"] = opts.ModelKey
		headers["X-Model-Source"] = modelSource
	}
	return SignedRequest{Body: encodedBody, Headers: headers}, nil
}

// PKCEPair generates a verifier and S256 challenge for device login.
func PKCEPair() (verifier, challenge string, err error) {
	length := 43 + 64
	raw := make([]byte, length)
	for i := range raw {
		_, err = rand.Read(raw[i : i+1])
		if err != nil {
			return "", "", err
		}
		raw[i] = StdB64Alphabet[int(raw[i])%64]
	}
	verifier = string(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func machineID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var buf bytes.Buffer
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf.WriteByte('-')
		}
		buf.WriteString(fmt.Sprintf("%02x", v))
	}
	return buf.String(), nil
}
