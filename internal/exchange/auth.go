package exchange

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"unicode"
)

func validPassword(password string) bool {
	if len(password) < 12 || len(password) > 72 {
		return false
	}
	var upper, lower, number, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			number = true
		case !unicode.IsSpace(r):
			symbol = true
		}
	}
	return upper && lower && number && symbol
}
func passwordHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
func keyHash(key string) string { sum := sha256.Sum256([]byte(key)); return hex.EncodeToString(sum[:]) }
func (e *Engine) BotRegister(official, username, requestID, secret string) (map[string]any, error) {
	app, openid, ok := strings.Cut(official, ":")
	if !ok || app == "" || openid == "" || strings.Contains(openid, ":") || len(official) > 256 || strings.ContainsAny(official, " \t\r\n\x00") {
		return nil, fail("INVALID_IDENTITY", "请提供有效官方机器人身份 appid:openid")
	}
	username = strings.TrimSpace(username)
	if len([]rune(username)) < 2 || len([]rune(username)) > 32 || strings.ContainsAny(username, "\r\n\x00") {
		return nil, fail("INVALID_USERNAME", "请发送 注册 <用户名>，用户名须为 2–32 个字符")
	}
	if len(requestID) < 8 || len(requestID) > 128 {
		return nil, fail("INVALID_REQUEST_ID", "注册操作需要有效幂等标识")
	}
	key := "register:" + keyHash(official) + ":" + requestID
	password := strongPassword()
	hash, err := passwordHash(password)
	if err != nil {
		return nil, err
	}
	sealed, err := sealSecret(secret, password)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = e.transaction(func(m *mutation) error {
		if a := e.accounts[e.byOfficial[official]]; a != nil {
			out = map[string]any{"status": "already_registered", "username": a.Username, "accountId": a.ID, "bindingRequired": a.BoundStockID == 0}
			if r := e.resets[key]; r != nil && r.ExpiresAt >= e.clock().UnixMilli() && r.PasswordVersion == a.PasswordVersion && a.ForcePasswordChange {
				if normalized(r.Username) != normalized(username) {
					return fail("IDEMPOTENCY_CONFLICT", "同一注册标识不能用于不同用户名")
				}
				plain, err := openSecret(secret, r.Ciphertext)
				if err != nil {
					return err
				}
				out["status"], out["password"], out["forcePasswordChange"] = "registered", plain, true
			}
			return nil
		}
		if e.nameTaken(username, 0) {
			return fail("NAME_TAKEN", "用户名已被使用")
		}
		id := m.meta.NextAccount
		m.meta.NextAccount++
		a := &Account{ID: id, MetricsSince: e.clock().UnixMilli(), IdentityCode: "MMEX-" + randomToken(18), Username: username, OfficialUserID: official, PasswordHash: hash, PasswordVersion: 1, ForcePasswordChange: true, CredentialsDelivered: true, Cash: m.meta.Settings.InitialCash, InitialCash: m.meta.Settings.InitialCash, Positions: map[int64]*Position{}, CreatedAt: e.clock().UnixMilli()}
		m.accounts[id] = a
		m.resets[key] = &ResetReceipt{Username: username, OfficialUserID: official, Ciphertext: sealed, PasswordVersion: 1, ExpiresAt: e.clock().UnixMilli() + 900000}
		m.changed = true
		out = map[string]any{"status": "registered", "username": username, "accountId": id, "password": password, "forcePasswordChange": true, "bindingRequired": true}
		return nil
	})
	return out, err
}
func (e *Engine) BotAccount(official string) (int64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if a := e.accounts[e.byOfficial[official]]; a != nil {
		if a.Disabled {
			return 0, fail("UNAUTHORIZED", "账户不可用")
		}
		return a.ID, nil
	}
	return 0, fail("NOT_REGISTERED", "请先向官方机器人发送 注册 <用户名>，再绑定股票")
}
func (e *Engine) Login(username, password string) (int64, int64, bool, error) {
	e.mu.RLock()
	var a *Account
	if x := e.accountByName(username); x != nil {
		copy := *x
		a = &copy
	}
	e.mu.RUnlock()
	if a == nil || a.Disabled || a.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return 0, 0, false, fail("INVALID_CREDENTIALS", "用户名或密码错误")
	}
	return a.ID, a.PasswordVersion, a.ForcePasswordChange, nil
}
func (e *Engine) CheckSession(id, version int64) (*Account, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a := e.accounts[id]
	if a == nil || a.Disabled || a.PasswordVersion != version {
		return nil, fail("UNAUTHORIZED", "登录已失效，请重新登录")
	}
	copy := *a
	copy.PasswordHash = ""
	copy.Positions = nil
	return &copy, nil
}
func (e *Engine) ChangePassword(id int64, current, password string) (int64, error) {
	if !validPassword(password) {
		return 0, fail("WEAK_PASSWORD", "密码须为 12–72 字符并包含大写、小写、数字及符号")
	}
	e.mu.RLock()
	old := e.accounts[id]
	hash := ""
	version := int64(0)
	if old != nil {
		hash = old.PasswordHash
		version = old.PasswordVersion
	}
	e.mu.RUnlock()
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return 0, fail("INVALID_CREDENTIALS", "当前密码不正确")
	}
	if current == password {
		return 0, fail("PASSWORD_UNCHANGED", "新密码不能与临时密码或当前密码相同")
	}
	newHash, err := passwordHash(password)
	if err != nil {
		return 0, err
	}
	err = e.transaction(func(m *mutation) error {
		a := m.account(id)
		if a == nil || a.PasswordVersion != version {
			return fail("UNAUTHORIZED", "密码已更新，请重新登录")
		}
		a.PasswordHash = newHash
		a.ForcePasswordChange = false
		a.PasswordVersion++
		version = a.PasswordVersion
		return nil
	})
	return version, err
}
func sealSecret(secret, value string) (string, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceBytes, err := base64.RawURLEncoding.DecodeString(randomToken(aead.NonceSize()))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonceBytes, nonceBytes, []byte(value), nil)), nil
}
func openSecret(secret, value string) (string, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < aead.NonceSize() {
		return "", fmt.Errorf("invalid sealed credential")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
	return string(plain), err
}
func (e *Engine) ResetPassword(official, requestID, secret string) (string, error) {
	id, err := e.BotAccount(official)
	if err != nil {
		return "", err
	}
	if len(requestID) < 8 || len(requestID) > 128 {
		return "", fail("INVALID_REQUEST_ID", "重置操作需要有效幂等标识")
	}
	key := idstr(id) + ":" + requestID
	e.mu.RLock()
	receipt := e.resets[key]
	currentVersion := e.accounts[id].PasswordVersion
	if receipt != nil {
		copy := *receipt
		receipt = &copy
	}
	e.mu.RUnlock()
	if receipt != nil {
		if receipt.OfficialUserID != official || receipt.ExpiresAt < e.clock().UnixMilli() {
			return "", fail("RESET_EXPIRED", "重置结果已过期，请创建新的二次确认操作")
		}
		if receipt.PasswordVersion != currentVersion {
			return "", fail("RESET_SUPERSEDED", "密码已再次更新，请使用最新密码或重新发起重置")
		}
		return openSecret(secret, receipt.Ciphertext)
	}
	password := strongPassword()
	hash, err := passwordHash(password)
	if err != nil {
		return "", err
	}
	sealed, err := sealSecret(secret, password)
	if err != nil {
		return "", err
	}
	err = e.transaction(func(m *mutation) error {
		if old := e.resets[key]; old != nil {
			if old.OfficialUserID != official {
				return fail("IDENTITY_MISMATCH", "身份不匹配")
			}
			if old.ExpiresAt < e.clock().UnixMilli() {
				return fail("RESET_EXPIRED", "重置结果已过期")
			}
			if old.PasswordVersion != m.readAccount(id).PasswordVersion {
				return fail("RESET_SUPERSEDED", "密码已再次更新，请使用最新密码或重新发起重置")
			}
			var err error
			password, err = openSecret(secret, old.Ciphertext)
			return err
		}
		a := m.account(id)
		a.PasswordHash = hash
		a.PasswordVersion++
		a.ForcePasswordChange = true
		a.CredentialsDelivered = true
		m.resets[key] = &ResetReceipt{PasswordVersion: a.PasswordVersion, OfficialUserID: official, Ciphertext: sealed, ExpiresAt: e.clock().UnixMilli() + 900000}
		return nil
	})
	return password, err
}
