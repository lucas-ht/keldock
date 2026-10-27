package access

import (
	"strings"
	"testing"
)

func TestPasswordAndCredentialBoundaries(t *testing.T) {
	password := "a long 🦊 phrase with spaces"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !VerifyPassword(first, password) || VerifyPassword(first, "incorrect password") {
		t.Fatal("password salt or verification failed")
	}
	for _, encoded := range []string{"", "$argon2id$v=19$m=4294967295,t=100,p=255$a$b", strings.Replace(first, "m=65536", "m=65536,extra=1", 1)} {
		if VerifyPassword(encoded, password) {
			t.Fatal("malformed or unbounded hash accepted")
		}
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
	one, two := NewToken("kds_"), NewToken("kds_")
	if one == two || strings.Contains(TokenHash(one), one) || !ValidCSRF(one, CSRF(one)) || ValidCSRF(two, CSRF(one)) {
		t.Fatal("credential or session anti-forgery boundary failed")
	}
}

func TestMachineCredentialsNeverAcquireHumanAuthority(t *testing.T) {
	principal := Principal{User: User{Status: "active", Role: "admin"}, Kind: "mcp", TokenProjects: []string{"sample"}}
	if principal.IsAdmin() || principal.CanOperate("sample") || principal.CanRead("other") || !principal.CanRead("sample") {
		t.Fatal("read credential escaped its narrowed authority")
	}
	principal.Kind = "session"
	if !principal.IsAdmin() || !principal.CanOperate("sample") {
		t.Fatal("human administrator lost authority")
	}
}
