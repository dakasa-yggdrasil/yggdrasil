package initcli

import "testing"

func TestContextNameFromServer(t *testing.T) {
	cases := []struct {
		server string
		want   string
	}{
		{"http://localhost:9080", "localhost"},
		{"https://yggdrasil.example.com", "yggdrasil-example-com"},
		{"https://core.platform.internal:443/api", "core-platform-internal"},
		{"", "remote"},
	}
	for _, tc := range cases {
		t.Run(tc.server, func(t *testing.T) {
			got := contextNameFromServer(tc.server)
			if got != tc.want {
				t.Errorf("contextNameFromServer(%q) = %q, want %q", tc.server, got, tc.want)
			}
		})
	}
}

func TestRandomPassword_LengthAndUniqueness(t *testing.T) {
	p1, err := randomPassword(24)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := randomPassword(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 24 || len(p2) != 24 {
		t.Fatalf("lengths = %d, %d, want 24", len(p1), len(p2))
	}
	if p1 == p2 {
		t.Fatal("two random passwords were identical — entropy source broken")
	}
}

func TestDefaults_FillsBlanksWithSafeChoices(t *testing.T) {
	opts := Options{}
	defaults(&opts)
	if opts.Dir == "" || opts.AdminUsername == "" || opts.AdminDisplayName == "" || opts.CoreImage == "" || opts.HTTPPort == 0 {
		t.Errorf("defaults left a critical field empty: %+v", opts)
	}
	if opts.ContextName != "local" {
		t.Errorf("ContextName default = %q, want local", opts.ContextName)
	}
}

func TestDefaults_DerivesContextNameFromServer(t *testing.T) {
	opts := Options{Server: "http://core.example:9080"}
	defaults(&opts)
	if opts.ContextName == "local" {
		t.Errorf("ContextName should not be 'local' when --server is given; got %q", opts.ContextName)
	}
	if opts.ContextName != "core-example" {
		t.Errorf("ContextName = %q, want core-example", opts.ContextName)
	}
}
