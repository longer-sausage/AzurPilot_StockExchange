package exchange

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var identityCodePattern = regexp.MustCompile(`^MMEX-[A-F0-9]{8}-[A-F0-9]{8}-[A-F0-9]{8}-[A-F0-9]{8}$`)

// 身份码与上市代码、实例身份及登录凭据独立，改名和赛季重置均不改变它。
func generateIdentityCode(used map[string]int64) (string, error) {
	for {
		var data [16]byte
		if _, err := rand.Read(data[:]); err != nil {
			return "", err
		}
		s := strings.ToUpper(hex.EncodeToString(data[:]))
		code := "MMEX-" + s[:8] + "-" + s[8:16] + "-" + s[16:24] + "-" + s[24:]
		if used[code] == 0 {
			return code, nil
		}
	}
}

func (e *Engine) newIdentityCode() (string, error) {
	return generateIdentityCode(e.identityCodes)
}

// 升级时在一个事务内补发旧账户身份码，并以 SQLite 唯一约束兜底。
func (e *Engine) migrateIdentityCodes() error {
	rows, err := e.db.Query("SELECT player_id,code FROM player_identity_codes")
	if err != nil {
		return err
	}
	registered := map[int64]string{}
	for rows.Next() {
		var id int64
		var code string
		if err = rows.Scan(&id, &code); err != nil {
			break
		}
		registered[id] = code
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if rowErr != nil {
		return rowErr
	}
	used := map[string]int64{}
	updates := map[int64]string{}
	for id, p := range e.players {
		code := p.IdentityCode
		if code == "" {
			code = registered[id]
		}
		if code != "" {
			if !identityCodePattern.MatchString(code) || used[code] != 0 || registered[id] != "" && registered[id] != code {
				return fmt.Errorf("账户 %d 的身份码无效或重复", id)
			}
			used[code] = id
			if p.IdentityCode != code || registered[id] != code {
				updates[id] = code
			}
		}
	}
	for id, p := range e.players {
		if p.IdentityCode == "" && registered[id] == "" {
			code, err := generateIdentityCode(used)
			if err != nil {
				return err
			}
			used[code], updates[id] = id, code
		}
	}
	return e.transaction(func() error {
		for id, code := range updates {
			e.touch(id).IdentityCode = code
		}
		return nil
	})
}
