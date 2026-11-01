package exchange

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"strings"
)

func canonicalName(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

// 老数据可能已有全半角等价名：保留原名直接登录；等价查询歧义时拒绝猜测。
func (e *Engine) accountByName(value string) *Account {
	if id := e.byUsername[normalized(value)]; id != 0 {
		return e.accounts[id]
	}
	var found *Account
	key := canonicalName(value)
	for _, a := range e.accounts {
		if canonicalName(a.Username) == key {
			if found != nil {
				return nil
			}
			found = a
		}
	}
	return found
}
func (e *Engine) nameTaken(value string, except int64) bool {
	key := canonicalName(value)
	for _, a := range e.accounts {
		if a.ID != except && canonicalName(a.Username) == key {
			return true
		}
	}
	return false
}
