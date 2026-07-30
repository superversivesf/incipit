package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/epub"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: incipit <command> [args]")
		fmt.Fprintln(os.Stderr, "commands: init, serve, parse, lookup, add, add-user, list-users, remove-user")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init":
		runInit()
	case "add-user":
		runAddUser(os.Args[2:])
	case "list-users":
		runListUsers()
	case "remove-user":
		runRemoveUser(os.Args[2:])
	case "serve":
		fmt.Println("serve: not yet implemented")
	case "parse":
		runParse(os.Args[2:])
	case "lookup":
		fmt.Println("lookup: not yet implemented")
	case "add":
		fmt.Println("add: not yet implemented")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func runInit() {
	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "error running migrations: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Database initialized at %s\n", cfg.DBPath)
}

func runAddUser(args []string) {
	fs := flag.NewFlagSet("add-user", flag.ExitOnError)
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password (plaintext)")
	role := fs.String("role", "user", "role (user or admin)")
	fs.Parse(args)

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: incipit add-user --username X --password Y [--role admin]")
		os.Exit(2)
	}

	hash, err := hashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error hashing password: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "error running migrations: %v\n", err)
		os.Exit(1)
	}

	if _, err := d.CreateUser(*username, hash, *role); err != nil {
		fmt.Fprintf(os.Stderr, "error creating user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User %s created (role: %s)\n", *username, *role)
}

func runListUsers() {
	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	users, err := d.ListUsers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing users: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%-20s %-10s %s\n", "USERNAME", "ROLE", "ID")
	for _, u := range users {
		fmt.Printf("%-20s %-10s %d\n", u.Username, u.Role, u.ID)
	}
}

func runRemoveUser(args []string) {
	fs := flag.NewFlagSet("remove-user", flag.ExitOnError)
	username := fs.String("username", "", "username to remove")
	fs.Parse(args)

	if *username == "" {
		fmt.Fprintln(os.Stderr, "usage: incipit remove-user --username X")
		os.Exit(2)
	}

	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.DeleteUser(*username); err != nil {
		fmt.Fprintf(os.Stderr, "error deleting user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User %s removed\n", *username)
}

func hashPassword(plaintext string) (string, error) {
	md5sum := md5.Sum([]byte(plaintext))
	md5hex := hex.EncodeToString(md5sum[:])

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(md5hex), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hashing: %w", err)
	}

	return string(bcryptHash), nil
}

func runParse(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: incipit parse <path>")
		os.Exit(2)
	}

	meta, err := epub.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing epub: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Title:      %s\n", meta.Title)
	fmt.Printf("Creator:    %s\n", meta.Creator)
	fmt.Printf("Identifier: %s\n", meta.Identifier)
	fmt.Printf("Language:   %s\n", meta.Language)
	fmt.Printf("Publisher:  %s\n", meta.Publisher)
	fmt.Printf("Date:       %s\n", meta.Date)
}
