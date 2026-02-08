package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"golang.org/x/term"
)

func runCreateAdmin(coreDB *db.CoreDB) {
	if len(os.Args) < 3 {
		log.Fatal("Usage: bobot create-admin <username>")
	}
	username := os.Args[2]

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		log.Fatalf("Failed to read password: %v", err)
	}

	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		log.Fatalf("Failed to read password confirmation: %v", err)
	}

	if string(passwordBytes) != string(confirmBytes) {
		log.Fatal("Passwords do not match")
	}

	hash, err := auth.HashPassword(string(passwordBytes))
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	user, err := coreDB.CreateUserFull(username, hash, username, "admin")
	if err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	coreDB.CreateMessage(db.BobotUserID, user.ID, "assistant", db.WelcomeMessage, db.WelcomeMessage)
	log.Printf("Created admin user: %s", username)
}
