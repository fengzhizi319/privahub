package migration

import (
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/sm3"
	"go.uber.org/zap"
)

func TestMigrateAuthSecurity_WrapsLegacyHashesAndDigestsTokens(t *testing.T) {
	db := setupMigrateTestDB(t)
	shaRow := hashPassword("Str0ng-Pass#1")
	sm3Row := sm3.SumHex([]byte("Java-Pass#1"))
	kdfRow, _ := auth.HashClientPassword(hashPassword("Already#1"))
	db.Create(&model.UserAccountsDO{Name: "go", PasswordHash: shaRow})
	db.Create(&model.UserAccountsDO{Name: "java", PasswordHash: sm3Row})
	db.Create(&model.UserAccountsDO{Name: "new", PasswordHash: kdfRow})
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1In0.sig"
	db.Create(&model.UserTokensDO{Name: "go", Token: jwt, SessionData: `{"token":"` + jwt + `","name":"go"}`})

	for i := 0; i < 2; i++ { // idempotent
		if err := MigrateAuthSecurity(db, zap.NewNop()); err != nil {
			t.Fatal(err)
		}
	}
	get := func(name string) string {
		var u model.UserAccountsDO
		db.Where("name = ?", name).First(&u)
		return u.PasswordHash
	}
	goHash, javaHash := get("go"), get("java")
	if auth.StoredPasswordFormat(goHash) != auth.FormatWrapped || auth.StoredPasswordFormat(javaHash) != auth.FormatWrapped {
		t.Fatalf("legacy rows not wrapped: %s / %s", goHash, javaHash)
	}
	if get("new") != kdfRow {
		t.Fatal("KDF rows must not be touched")
	}
	// The leaked raw value is no longer a credential; the real password is.
	if auth.VerifyStoredPassword(goHash, auth.PasswordCandidates{SHA256: goHash}).OK {
		t.Fatal("wrapped row must not verify against itself")
	}
	if !auth.VerifyStoredPassword(goHash, auth.PasswordCandidates{SHA256: hashPassword("Str0ng-Pass#1")}).OK {
		t.Fatal("wrapped SHA-256 row must verify with the client hash")
	}
	if !auth.VerifyStoredPassword(javaHash, auth.PasswordCandidates{SHA256: hashPassword("Java-Pass#1"), SM3: sm3.SumHex([]byte("Java-Pass#1"))}).OK {
		t.Fatal("wrapped Java SM3 row must verify with passwordHashSm3")
	}

	var row model.UserTokensDO
	db.Where("name = ?", "go").First(&row)
	if row.Token != auth.TokenDigest(jwt) || strings.Contains(row.SessionData, jwt) {
		t.Fatalf("token not digested / session_data still has the token: %+v", row)
	}
}
