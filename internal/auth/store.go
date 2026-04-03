package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// TokenType is the authentication scheme.
type TokenType string

const (
	TokenBearer TokenType = "bearer"
	TokenAPIKey TokenType = "apikey"
	TokenBasic  TokenType = "basic"
)

// Context holds a named authentication context.
type Context struct {
	Name      string    `json:"name"`
	Type      TokenType `json:"type"`
	Token     string    `json:"token"`
	HeaderKey string    `json:"header_key,omitempty"` // for apikey type
}

// Store manages auth contexts in an AES-256-GCM encrypted file.
type Store struct {
	path   string
	key    []byte
	Active string `json:"active"`
}

// NewStore opens (or creates) the auth store using a machine-derived key.
// The key is derived from the provided passphrase via SHA-256.
func NewStore(passphrase string) (*Store, error) {
	storePath, err := xdg.DataFile("curlx/auth.enc")
	if err != nil {
		return nil, fmt.Errorf("resolving auth store path: %w", err)
	}

	hash := sha256.Sum256([]byte(passphrase))
	return &Store{
		path: storePath,
		key:  hash[:],
	}, nil
}

type storeData struct {
	Active   string             `json:"active"`
	Contexts map[string]Context `json:"contexts"`
}

func (s *Store) load() (*storeData, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &storeData{Contexts: map[string]Context{}}, nil
	}
	if err != nil {
		return nil, err
	}

	plain, err := s.decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypting auth store: %w", err)
	}

	var sd storeData
	if err := json.Unmarshal(plain, &sd); err != nil {
		return nil, fmt.Errorf("parsing auth store: %w", err)
	}
	if sd.Contexts == nil {
		sd.Contexts = map[string]Context{}
	}
	return &sd, nil
}

func (s *Store) save(sd *storeData) error {
	plain, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	enc, err := s.encrypt(plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	return os.WriteFile(s.path, enc, 0600)
}

// Save adds or updates a named auth context.
func (s *Store) Save(ctx Context) error {
	sd, err := s.load()
	if err != nil {
		return err
	}
	sd.Contexts[ctx.Name] = ctx
	return s.save(sd)
}

// List returns all stored auth contexts.
func (s *Store) List() ([]Context, error) {
	sd, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Context, 0, len(sd.Contexts))
	for _, c := range sd.Contexts {
		out = append(out, c)
	}
	return out, nil
}

// SetActive marks a named context as active.
func (s *Store) SetActive(name string) error {
	sd, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := sd.Contexts[name]; !ok {
		return fmt.Errorf("auth context %q not found", name)
	}
	sd.Active = name
	return s.save(sd)
}

// Active returns the currently active auth context.
func (s *Store) GetActive() (*Context, error) {
	sd, err := s.load()
	if err != nil {
		return nil, err
	}
	if sd.Active == "" {
		return nil, nil
	}
	ctx, ok := sd.Contexts[sd.Active]
	if !ok {
		return nil, fmt.Errorf("active context %q not found", sd.Active)
	}
	return &ctx, nil
}

// Delete removes a named auth context.
func (s *Store) Delete(name string) error {
	sd, err := s.load()
	if err != nil {
		return err
	}
	delete(sd.Contexts, name)
	if sd.Active == name {
		sd.Active = ""
	}
	return s.save(sd)
}

func (s *Store) encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (s *Store) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
