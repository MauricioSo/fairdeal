package auth

import "testing"

func TestPasswordVerifier(t *testing.T) {
	p := "a long randomly chosen test password"
	first, e := HashPassword(p)
	if e != nil {
		t.Fatal(e)
	}
	second, e := HashPassword(p)
	if e != nil {
		t.Fatal(e)
	}
	if first == second {
		t.Fatal("password salt was reused")
	}
	if !VerifyPassword(p, first) || VerifyPassword("incorrect", first) {
		t.Fatal("password verification failed")
	}
	for _, bad := range []string{"", "plain-password", "pbkdf2-sha256-600000$00$00", "pbkdf2-sha256-1$00$00"} {
		if ValidHash(bad) || VerifyPassword(p, bad) {
			t.Fatal("malformed verifier accepted")
		}
	}
	if _, e := HashPassword("short"); e == nil {
		t.Fatal("weak generated password accepted")
	}
}
