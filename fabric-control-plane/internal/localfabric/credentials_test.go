package localfabric

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCAClientPasswordsStayInProtectedConfig(t *testing.T) {
	for _, args := range [][]string{{"enroll", "-u", "https://admin:unique-password@ca:7054"}, {"register", "--id.name", "peer1", "--id.secret", "unique-password"}} {
		config, argv, err := protectedCAClientConfig(args)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(argv, " "), "unique-password") {
			t.Fatal("password in argv")
		}
		raw, _ := json.Marshal(config)
		if !strings.Contains(string(raw), "unique-password") {
			t.Fatal("missing private config")
		}
	}
	if _, _, err := protectedCAClientConfig([]string{"--id.secret=unsafe"}); err == nil {
		t.Fatal("alternate secret flag accepted")
	}
}
func TestComposeNeverContainsEnrollmentPasswords(t *testing.T) {
	passwords := map[string]string{"org1": "secret-one", "org2": "secret-two", "orderers": "secret-three"}
	raw, _ := json.Marshal(compose(t.TempDir(), passwords))
	for _, password := range passwords {
		if strings.Contains(string(raw), password) {
			t.Fatal("password persisted in compose/container arguments")
		}
	}
	bootstrap, _ := json.Marshal(caBootstrap("secret-one"))
	if !strings.Contains(string(bootstrap), "secret-one") {
		t.Fatal("CA bootstrap credential missing from private server configuration")
	}
}
