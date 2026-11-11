package web

import "testing"

func TestEmailAllowed(t *testing.T) {
	a := &authConfig{domains: []string{"itera.ac.id"}}
	cases := map[string]bool{
		"budi@itera.ac.id":             true,
		"budi.123@student.itera.ac.id": true,
		"BUDI@Student.ITERA.ac.id":     true,
		"budi@gmail.com":               false,
		"budi@fakeitera.ac.id":         false, // bukan subdomain
		"budi@itera.ac.id.evil.com":    false,
		"budi@student.itera.ac.id.co":  false,
		"itera.ac.id":                  false, // tanpa @
		"":                             false,
	}
	for email, want := range cases {
		if got := a.emailAllowed(email); got != want {
			t.Errorf("emailAllowed(%q) = %v, mau %v", email, got, want)
		}
	}
}
