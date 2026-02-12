package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
)

func runGenerateVAPIDKeys() {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate VAPID keys: %v", err)
	}

	b64 := base64.RawURLEncoding
	fmt.Printf("BOBOT_VAPID_PUBLIC_KEY=%s\n", b64.EncodeToString(key.PublicKey().Bytes()))
	fmt.Printf("BOBOT_VAPID_PRIVATE_KEY=%s\n", b64.EncodeToString(key.Bytes()))
	fmt.Printf("BOBOT_VAPID_SUBJECT=mailto:you@example.com\n")
}
