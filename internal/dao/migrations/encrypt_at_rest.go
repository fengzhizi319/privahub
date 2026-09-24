package migration

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	pcrypto "github.com/fengzhizi319/privahub/pkg/crypto"
)

const secretCallbackName = "privahub:seal_secret_map_values"

// RegisterSecretCallbacks seals values of `serializer:secret` columns passed
// through map-based writes (Updates(map), Update(col, v), Create(map)), which
// GORM does not route through field serializers. Idempotent.
func RegisterSecretCallbacks(db *gorm.DB) error {
	cb := db.Callback()
	if err := cb.Update().Before("gorm:update").Replace(secretCallbackName, sealMapDest); err != nil {
		return err
	}
	return cb.Create().Before("gorm:create").Replace(secretCallbackName, sealMapDest)
}

func secretField(sch *schema.Schema, key string) bool {
	if sch == nil {
		return false
	}
	f := sch.LookUpField(key)
	return f != nil && strings.EqualFold(f.TagSettings["SERIALIZER"], model.SecretSerializer)
}

func isSecretColumn(table, col string) bool {
	for _, c := range model.SecretColumns {
		if c.Table == table && c.Column == col {
			return true
		}
	}
	return false
}

func sealMapDest(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement == nil {
		return
	}
	st := tx.Statement
	m, ok := st.Dest.(map[string]interface{})
	if !ok {
		if pm, ok2 := st.Dest.(*map[string]interface{}); ok2 && pm != nil {
			m, ok = *pm, true
		}
	}
	if !ok || len(m) == 0 {
		return
	}
	var out map[string]interface{}
	for k, v := range m {
		if !secretField(st.Schema, k) && !isSecretColumn(st.Table, k) {
			continue
		}
		s, isStr := v.(string)
		if !isStr {
			continue
		}
		sealed, err := pcrypto.Seal(s)
		if err != nil {
			_ = tx.AddError(fmt.Errorf("seal %s: %w", k, err))
			return
		}
		if out == nil {
			out = make(map[string]interface{}, len(m))
			for k2, v2 := range m {
				out[k2] = v2
			}
		}
		out[k] = sealed
	}
	if out != nil {
		st.Dest = out
	}
}

// EncryptSecretsAtRest registers the secret callbacks and then encrypts every
// legacy plaintext value of model.SecretColumns, re-encrypting values sealed
// with a non-active key version (key rotation). It is idempotent and a no-op
// without a keyring. Raw SQL is used so the rewrite neither re-triggers the
// data-sync producer nor skips soft-deleted rows.
func EncryptSecretsAtRest(ctx context.Context, db *gorm.DB, log *zap.Logger) error {
	if err := RegisterSecretCallbacks(db); err != nil {
		return err
	}
	kr := pcrypto.Default()
	if kr == nil {
		return nil
	}
	if log == nil {
		log = zap.NewNop()
	}
	for _, c := range model.SecretColumns {
		if !db.Migrator().HasTable(c.Table) || !db.Migrator().HasColumn(c.Table, c.Column) {
			continue
		}
		var rows []struct {
			ID    int64
			Value string
		}
		q := fmt.Sprintf("SELECT id, %s AS value FROM %s WHERE %s IS NOT NULL AND %s <> ''",
			quote(db, c.Column), quote(db, c.Table), quote(db, c.Column), quote(db, c.Column))
		if err := db.WithContext(ctx).Raw(q).Scan(&rows).Error; err != nil {
			return fmt.Errorf("scan %s.%s: %w", c.Table, c.Column, err)
		}
		migrated, failed := 0, 0
		for _, r := range rows {
			if !kr.NeedsRotation(r.Value) {
				continue
			}
			plain := r.Value
			if pcrypto.IsEncrypted(plain) {
				p, err := kr.Decrypt(plain)
				if err != nil {
					failed++
					log.Error("at-rest: cannot decrypt value for re-encryption (missing old key version?)",
						zap.String("table", c.Table), zap.String("column", c.Column), zap.Int64("id", r.ID), zap.Error(err))
					continue
				}
				plain = p
			}
			sealed, err := kr.Encrypt(plain)
			if err != nil {
				return err
			}
			u := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", quote(db, c.Table), quote(db, c.Column))
			if err := db.WithContext(ctx).Exec(u, sealed, r.ID).Error; err != nil {
				return fmt.Errorf("update %s.%s id=%d: %w", c.Table, c.Column, r.ID, err)
			}
			migrated++
		}
		if migrated > 0 || failed > 0 {
			log.Info("at-rest: encrypted secret column", zap.String("table", c.Table), zap.String("column", c.Column),
				zap.Int("migrated", migrated), zap.Int("failed", failed), zap.String("key_version", kr.ActiveVersion()))
		}
	}
	return nil
}

func quote(db *gorm.DB, name string) string {
	var b strings.Builder
	db.Dialector.QuoteTo(&b, name)
	return b.String()
}
