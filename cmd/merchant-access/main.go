// merchant-access creates a strong password and its verifier in a private local file.
// It never prints credentials, and refuses to overwrite an existing file.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nashpay/nashpay/internal/auth"
	"os"
)

func main() {
	email := flag.String("email", "merchant@fairdeal.demo", "Merchant login email")
	output := flag.String("out", "", "Private output JSON path outside the source repository")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(1)
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		panic(err)
	}
	password := base64.RawURLEncoding.EncodeToString(random)
	verifier, err := auth.HashPassword(password)
	if err != nil {
		panic(err)
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot create private credentials file")
		os.Exit(1)
	}
	defer file.Close()
	if err = json.NewEncoder(file).Encode(map[string]string{"email": *email, "password": password, "password_hash": verifier}); err != nil {
		panic(err)
	}
	fmt.Println("Created private merchant credentials. Configure MERCHANT_EMAIL and MERCHANT_PASSWORD_HASH; never publish the output file.")
}
