package exchange

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

type InstanceBinding struct {
	Key        string `json:"key"`
	InstanceID string `json:"instanceId"`
	PublicKey  string `json:"publicKey"`
}

// 签名只确认实例身份及请求完整性，不证明行动力或游戏数据真实。
type InstanceReport struct {
	InstanceID   string `json:"instanceId"`
	PublicKey    string `json:"publicKey"`
	ActionPoints int64  `json:"actionPoints"`
	ObservedAt   int64  `json:"observedAt"`
	IssuedAt     int64  `json:"issuedAt"`
	Signature    string `json:"signature"`
}

var instancePattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func reportCanonical(p InstanceReport) string {
	return fmt.Sprintf("mmex-instance-v1\n%s\n%s\n%d\n%d\n%d\n", p.InstanceID, p.PublicKey, p.ActionPoints, p.ObservedAt, p.IssuedAt)
}
func verifyInstance(p InstanceReport, now time.Time) (*InstanceBinding, error) {
	if !instancePattern.MatchString(p.InstanceID) || p.ActionPoints < 0 || p.ActionPoints > 1_000_000 || p.ObservedAt < 0 {
		return nil, fail("INVALID_INSTANCE", "实例身份或行动力字段格式无效")
	}
	if p.IssuedAt > now.Unix()+60 || p.IssuedAt < now.Unix()-120 {
		return nil, fail("STALE_INSTANCE", "实例认证请求已过期，请重试")
	}
	pub, err := base64.StdEncoding.DecodeString(p.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, fail("INVALID_INSTANCE", "实例公钥无效")
	}
	sig, err := base64.StdEncoding.DecodeString(p.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), []byte(reportCanonical(p)), sig) {
		return nil, fail("INVALID_INSTANCE", "实例签名无效")
	}
	key := sha256.Sum256([]byte(p.PublicKey + "\n" + p.InstanceID))
	return &InstanceBinding{hex.EncodeToString(key[:]), p.InstanceID, p.PublicKey}, nil
}
