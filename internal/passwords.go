package libro

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
)

const (
	passwordSaltSetting     = "password_vault_salt"
	passwordVerifierSetting = "password_vault_verifier"
	passwordPBKDF2Rounds    = 210000
)

// PasswordEntry is a decrypted password vault entry.
type PasswordEntry struct {
	ID       int64
	Name     string
	URL      string
	Username string
	Password string
	Note     string
}

// PasswordEntrySummary is safe to expose to the search UI after vault unlock.
type PasswordEntrySummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Note string `json:"note"`
}

var passwordVault = struct {
	sync.RWMutex
	key []byte
}{}

func passwordVaultConfigured() bool {
	return DBGetSetting(passwordSaltSetting, "") != "" && DBGetSetting(passwordVerifierSetting, "") != ""
}

func passwordVaultUnlocked() bool {
	passwordVault.RLock()
	defer passwordVault.RUnlock()
	return len(passwordVault.key) == 32
}

func passwordVaultStatusJS() string {
	return fmt.Sprintf("window.__libroPasswordConfigured=%t;window.__libroPasswordUnlocked=%t;", passwordVaultConfigured(), passwordVaultUnlocked())
}

func passwordEntriesJS() string {
	entries, err := DBLoadPasswordEntrySummaries()
	if err != nil {
		log.Printf("passwords: load summaries: %v", err)
		entries = nil
	}
	b, _ := jsonMarshal(entries)
	return fmt.Sprintf("window.__libroPasswordEntries=%s;", string(b))
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func passwordCurrentKey() ([]byte, bool) {
	passwordVault.RLock()
	defer passwordVault.RUnlock()
	if len(passwordVault.key) != 32 {
		return nil, false
	}
	key := append([]byte(nil), passwordVault.key...)
	return key, true
}

func passwordSetCurrentKey(key []byte) {
	passwordVault.Lock()
	defer passwordVault.Unlock()
	passwordVault.key = append([]byte(nil), key...)
}

func setupPasswordVault(master string) error {
	master = strings.TrimSpace(master)
	if len(master) < 8 {
		return errors.New("master password must be at least 8 characters")
	}
	if passwordVaultConfigured() {
		return errors.New("password vault is already configured")
	}

	salt, err := randomBytes(16)
	if err != nil {
		return err
	}
	key := derivePasswordKey(master, salt)
	verifier, err := encryptStringWithKey(key, "libro-password-vault")
	if err != nil {
		return err
	}

	DBSetSetting(passwordSaltSetting, base64.StdEncoding.EncodeToString(salt))
	DBSetSetting(passwordVerifierSetting, verifier)
	passwordSetCurrentKey(key)
	return nil
}

func unlockPasswordVault(master string) error {
	master = strings.TrimSpace(master)
	saltB64 := DBGetSetting(passwordSaltSetting, "")
	verifier := DBGetSetting(passwordVerifierSetting, "")
	if saltB64 == "" || verifier == "" {
		return errors.New("password vault is not configured")
	}
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return errors.New("password vault metadata is invalid")
	}
	key := derivePasswordKey(master, salt)
	plain, err := decryptStringWithKey(key, verifier)
	if err != nil || plain != "libro-password-vault" {
		return errors.New("incorrect master password")
	}
	passwordSetCurrentKey(key)
	return nil
}

func derivePasswordKey(master string, salt []byte) []byte {
	return pbkdf2SHA256([]byte(master), salt, passwordPBKDF2Rounds, 32)
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hashLen := sha256.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := cryptoRand.Read(b)
	return b, err
}

func encryptStringWithKey(key []byte, plain string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func decryptStringWithKey(key []byte, encoded string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
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
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("ciphertext is too short")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func DBAddPasswordEntry(entry PasswordEntry) error {
	key, ok := passwordCurrentKey()
	if !ok {
		return errors.New("password vault is locked")
	}
	nameCipher, err := encryptStringWithKey(key, strings.TrimSpace(entry.Name))
	if err != nil {
		return err
	}
	urlCipher, err := encryptStringWithKey(key, strings.TrimSpace(entry.URL))
	if err != nil {
		return err
	}
	usernameCipher, err := encryptStringWithKey(key, entry.Username)
	if err != nil {
		return err
	}
	passwordCipher, err := encryptStringWithKey(key, entry.Password)
	if err != nil {
		return err
	}
	noteCipher, err := encryptStringWithKey(key, strings.TrimSpace(entry.Note))
	if err != nil {
		return err
	}

	dbMu.Lock()
	defer dbMu.Unlock()
	_, err = db.Exec(
		"INSERT INTO password_entries (name_cipher, url_cipher, username_cipher, password_cipher, note_cipher, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)",
		nameCipher, urlCipher, usernameCipher, passwordCipher, noteCipher,
	)
	if err != nil && strings.Contains(err.Error(), "no column named note_cipher") {
		_, err = db.Exec(
			"INSERT INTO password_entries (name_cipher, url_cipher, username_cipher, password_cipher, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)",
			nameCipher, urlCipher, usernameCipher, passwordCipher,
		)
	}
	if err != nil {
		log.Printf("db: add password entry: %v", err)
	}
	return err
}

func DBLoadPasswordEntry(id int64) (PasswordEntry, error) {
	key, ok := passwordCurrentKey()
	if !ok {
		return PasswordEntry{}, errors.New("password vault is locked")
	}

	dbMu.Lock()
	defer dbMu.Unlock()
	var entry PasswordEntry
	var nameCipher, urlCipher, usernameCipher, passwordCipher, noteCipher string
	err := db.QueryRow("SELECT id, name_cipher, url_cipher, username_cipher, password_cipher, note_cipher FROM password_entries WHERE id = ?", id).
		Scan(&entry.ID, &nameCipher, &urlCipher, &usernameCipher, &passwordCipher, &noteCipher)
	if err != nil && strings.Contains(err.Error(), "no such column: note_cipher") {
		err = db.QueryRow("SELECT id, name_cipher, url_cipher, username_cipher, password_cipher FROM password_entries WHERE id = ?", id).
			Scan(&entry.ID, &nameCipher, &urlCipher, &usernameCipher, &passwordCipher)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return PasswordEntry{}, errors.New("password entry not found")
		}
		return PasswordEntry{}, err
	}
	if entry.Name, err = decryptStringWithKey(key, nameCipher); err != nil {
		return PasswordEntry{}, err
	}
	if entry.URL, err = decryptStringWithKey(key, urlCipher); err != nil {
		return PasswordEntry{}, err
	}
	if entry.Username, err = decryptStringWithKey(key, usernameCipher); err != nil {
		return PasswordEntry{}, err
	}
	if entry.Password, err = decryptStringWithKey(key, passwordCipher); err != nil {
		return PasswordEntry{}, err
	}
	if noteCipher != "" {
		if entry.Note, err = decryptStringWithKey(key, noteCipher); err != nil {
			return PasswordEntry{}, err
		}
	}
	return entry, nil
}

func DBLoadPasswordEntrySummaries() ([]PasswordEntrySummary, error) {
	key, ok := passwordCurrentKey()
	if !ok {
		return nil, nil
	}

	dbMu.Lock()
	defer dbMu.Unlock()
	withNote := true
	rows, err := db.Query("SELECT id, name_cipher, url_cipher, note_cipher FROM password_entries ORDER BY updated_at DESC, id DESC")
	if err != nil && strings.Contains(err.Error(), "no such column: note_cipher") {
		withNote = false
		rows, err = db.Query("SELECT id, name_cipher, url_cipher FROM password_entries ORDER BY updated_at DESC, id DESC")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PasswordEntrySummary
	for rows.Next() {
		var id int64
		var nameCipher, urlCipher, noteCipher string
		if withNote {
			err = rows.Scan(&id, &nameCipher, &urlCipher, &noteCipher)
		} else {
			err = rows.Scan(&id, &nameCipher, &urlCipher)
		}
		if err != nil {
			continue
		}
		name, nameErr := decryptStringWithKey(key, nameCipher)
		url, urlErr := decryptStringWithKey(key, urlCipher)
		note := ""
		var noteErr error
		if noteCipher != "" {
			note, noteErr = decryptStringWithKey(key, noteCipher)
		}
		if nameErr != nil || urlErr != nil || noteErr != nil {
			continue
		}
		out = append(out, PasswordEntrySummary{ID: id, Name: name, URL: url, Note: note})
	}
	return out, rows.Err()
}

func DBUpdatePasswordEntry(entry PasswordEntry) error {
	key, ok := passwordCurrentKey()
	if !ok {
		return errors.New("password vault is locked")
	}
	nameCipher, err := encryptStringWithKey(key, strings.TrimSpace(entry.Name))
	if err != nil {
		return err
	}
	urlCipher, err := encryptStringWithKey(key, strings.TrimSpace(entry.URL))
	if err != nil {
		return err
	}
	usernameCipher, err := encryptStringWithKey(key, entry.Username)
	if err != nil {
		return err
	}
	passwordCipher, err := encryptStringWithKey(key, entry.Password)
	if err != nil {
		return err
	}
	noteCipher, err := encryptStringWithKey(key, strings.TrimSpace(entry.Note))
	if err != nil {
		return err
	}

	dbMu.Lock()
	defer dbMu.Unlock()
	res, err := db.Exec(
		"UPDATE password_entries SET name_cipher = ?, url_cipher = ?, username_cipher = ?, password_cipher = ?, note_cipher = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		nameCipher, urlCipher, usernameCipher, passwordCipher, noteCipher, entry.ID,
	)
	if err != nil && strings.Contains(err.Error(), "no such column: note_cipher") {
		res, err = db.Exec(
			"UPDATE password_entries SET name_cipher = ?, url_cipher = ?, username_cipher = ?, password_cipher = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			nameCipher, urlCipher, usernameCipher, passwordCipher, entry.ID,
		)
	}
	if err != nil {
		log.Printf("db: update password entry: %v", err)
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errors.New("password entry not found")
	}
	return nil
}

func DBTouchPasswordEntry(id int64) {
	dbMu.Lock()
	defer dbMu.Unlock()
	_, err := db.Exec("UPDATE password_entries SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	if err != nil {
		log.Printf("db: touch password entry: %v", err)
	}
}

func DBDeletePasswordEntry(id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	_, err := db.Exec("DELETE FROM password_entries WHERE id = ?", id)
	if err != nil {
		log.Printf("db: delete password entry: %v", err)
	}
	return err
}
