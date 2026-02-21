package helperconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	dbmodel "shellman/cli/internal/db"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	cfgKeyOpenAIEndpoint  = "helper_openai_endpoint"
	cfgKeyOpenAIModel     = "helper_openai_model"
	cfgKeyOpenAIAPIKeyEnc = "helper_openai_api_key_enc"
	secretKeySize         = 32
)

type OpenAIConfig struct {
	Endpoint  string
	Model     string
	APIKey    string
	APIKeySet bool
}

type Store struct {
	db  *gorm.DB
	key []byte
}

// NewStore uses the shared global DB. Caller must not close the db.
func NewStore(db *gorm.DB, secretPath string) (*Store, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	key, err := loadOrCreateSecretKey(secretPath)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, key: key}, nil
}

// Close is a no-op; DB is process-wide and must not be closed by the store.
func (s *Store) Close() error {
	return nil
}

func (s *Store) SaveOpenAI(cfg OpenAIConfig) error {
	if s == nil || s.db == nil {
		return errors.New("helper config store is not initialized")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := upsertValue(tx, cfgKeyOpenAIEndpoint, strings.TrimSpace(cfg.Endpoint)); err != nil {
			return err
		}
		if err := upsertValue(tx, cfgKeyOpenAIModel, strings.TrimSpace(cfg.Model)); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.APIKey) == "" {
			return nil
		}
		enc, err := encryptAPIKey(cfg.APIKey, s.key)
		if err != nil {
			return err
		}
		return upsertValue(tx, cfgKeyOpenAIAPIKeyEnc, enc)
	})
}

func (s *Store) LoadOpenAI() (OpenAIConfig, error) {
	if s == nil || s.db == nil {
		return OpenAIConfig{}, errors.New("helper config store is not initialized")
	}

	endpoint, _ := s.rawValueOptional(cfgKeyOpenAIEndpoint)
	model, _ := s.rawValueOptional(cfgKeyOpenAIModel)
	encAPIKey, _ := s.rawValueOptional(cfgKeyOpenAIAPIKeyEnc)

	out := OpenAIConfig{
		Endpoint: strings.TrimSpace(endpoint),
		Model:    strings.TrimSpace(model),
	}
	if strings.TrimSpace(encAPIKey) == "" {
		return out, nil
	}
	plain, err := decryptAPIKey(encAPIKey, s.key)
	if err != nil {
		return OpenAIConfig{}, err
	}
	out.APIKey = plain
	out.APIKeySet = true
	return out, nil
}

func (s *Store) rawValue(key string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("helper config store is not initialized")
	}
	var row dbmodel.Config
	if err := s.db.Model(&dbmodel.Config{}).Select("value").Where("key = ?", key).Take(&row).Error; err != nil {
		return "", err
	}
	return row.Value, nil
}

func (s *Store) rawValueOptional(key string) (string, bool) {
	v, err := s.rawValue(key)
	if err == nil {
		return v, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	return "", false
}

func upsertValue(tx *gorm.DB, key, value string) error {
	now := time.Now().UTC().Unix()
	row := dbmodel.Config{
		Key:       key,
		Value:     value,
		UpdatedAt: now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"value":      row.Value,
			"updated_at": row.UpdatedAt,
		}),
	}).Create(&row).Error
}

func loadOrCreateSecretKey(secretPath string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o755); err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(secretPath); err == nil {
		if len(b) != secretKeySize {
			return nil, fmt.Errorf("invalid helper secret size: got %d", len(b))
		}
		return b, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, secretKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(secretPath, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptAPIKey(plain string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

func decryptAPIKey(enc string, key []byte) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(blob) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := blob[:nonceSize], blob[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
