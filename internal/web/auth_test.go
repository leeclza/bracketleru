package web

import "testing"

func TestEmailAllowed(t *testing.T) {
	a := &authConfig{domains: []string{"student.itera.ac.id"}}
	cases := map[string]bool{
		"budi.123@student.itera.ac.id": true,
		"BUDI@Student.ITERA.ac.id":     true,
		"dosen@itera.ac.id":            false, // staf, bukan student
		"budi@x.student.itera.ac.id":   false, // subdomain lain
		"budi@gmail.com":               false,
		"budi@fakestudent.itera.ac.id": false,
		"budi@student.itera.ac.id.com": false,
		"student.itera.ac.id":          false, // tanpa @
		"":                             false,
	}
	for email, want := range cases {
		if got := a.emailAllowed(email); got != want {
			t.Errorf("emailAllowed(%q) = %v, mau %v", email, got, want)
		}
	}
}

func TestValidUsername(t *testing.T) {
	ok := []string{"leon", "leon_123", "a.b_c", "abc"}
	bad := []string{"ab", "Leon", "leon!", "spasi kosong", "abcdefghijklmnopqrstu", ""}
	for _, s := range ok {
		if !validUsername.MatchString(s) {
			t.Errorf("%q harusnya valid", s)
		}
	}
	for _, s := range bad {
		if validUsername.MatchString(s) {
			t.Errorf("%q harusnya tidak valid", s)
		}
	}
}
