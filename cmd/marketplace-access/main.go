// Provision five distinct demo merchant accounts. Never prints secrets.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nashpay/nashpay/internal/auth"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	output := flag.String("out-dir", "", "Private directory outside the source checkout")
	flag.Parse()
	if *output == "" {
		fail("-out-dir is required")
	}
	cwd, err := os.Getwd()
	if err != nil {
		fail("cannot resolve working directory")
	}
	absolute, err := filepath.Abs(*output)
	if err != nil {
		fail("cannot resolve output path")
	}
	relative, err := filepath.Rel(cwd, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		fail("credentials must be generated outside the source checkout")
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		fail("cannot create private directory")
	}
	credentials := map[string]map[string]string{}
	verifiers := map[string]map[string]string{}
	for _, id := range []string{"alpine", "nordic", "glacier", "sierra", "polaris"} {
		random := make([]byte, 24)
		if _, err = rand.Read(random); err != nil {
			fail("random generation failed")
		}
		password := base64.RawURLEncoding.EncodeToString(random)
		hash, e := auth.HashPassword(password)
		if e != nil {
			fail("password hashing failed")
		}
		email := id + "@fairdeal.demo"
		credentials[id] = map[string]string{"email": email, "password": password}
		verifiers[id] = map[string]string{"email": email, "password_hash": hash}
	}
	writePrivate(filepath.Join(absolute, "merchant-access-private.json"), credentials)
	writePrivate(filepath.Join(absolute, "merchant-verifiers-private.json"), verifiers)
	fmt.Println("Created private access and verifier files. Set MERCHANTS_JSON from the verifier file only. Never publish either file.")
}
func writePrivate(path string, value any) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fail("refusing to overwrite private file")
	}
	defer file.Close()
	if json.NewEncoder(file).Encode(value) != nil {
		fail("cannot write private file")
	}
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
