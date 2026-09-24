package model

import (
	"context"
	"fmt"
	"reflect"

	"gorm.io/gorm/schema"

	pcrypto "github.com/fengzhizi319/privahub/pkg/crypto"
)

// SecretSerializer is the GORM serializer name for columns encrypted at rest
// (`gorm:"serializer:secret"`). Values are sealed with the process keyring
// (pkg/crypto, SM4-GCM + HKDF-SM3) on write and opened on read; legacy
// plaintext values are read as-is and encrypted by the startup migration
// (migration.EncryptSecretsAtRest). Without a keyring (dev/test without
// crypto keys) values pass through unchanged.
//
// Map-based updates (Updates(map)/Update(col, v)) bypass GORM serializers;
// RegisterSecretCallbacks seals those too.
const SecretSerializer = "secret"

// SecretColumn names a table column stored with SecretSerializer.
type SecretColumn struct {
	Table  string
	Column string
}

// SecretColumns lists every encrypted column (kept in sync with the model tags).
var SecretColumns = []SecretColumn{
	{"node", "token"},
	{"node", "inst_token"},
	{"datasource", "connection_info"},
	{"certificate", "key_pem"},
}

type secretSerializer struct{}

func (secretSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var s string
	switch v := dbValue.(type) {
	case nil:
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("secret column %s: unsupported db type %T", field.DBName, dbValue)
	}
	pt, err := pcrypto.Open(s)
	if err != nil {
		return fmt.Errorf("secret column %s: %w", field.DBName, err)
	}
	field.ReflectValueOf(ctx, dst).SetString(pt)
	return nil
}

func (secretSerializer) Value(_ context.Context, field *schema.Field, _ reflect.Value, fieldValue interface{}) (interface{}, error) {
	s, ok := fieldValue.(string)
	if !ok {
		return nil, fmt.Errorf("secret column %s: expected string, got %T", field.DBName, fieldValue)
	}
	return pcrypto.Seal(s)
}

func init() {
	schema.RegisterSerializer(SecretSerializer, secretSerializer{})
}
