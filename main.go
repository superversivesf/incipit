package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/epub"
	"github.com/jason/incipit/internal/lookup"
	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/server"
	"github.com/jason/incipit/internal/storage"
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
		runServe()
	case "parse":
		runParse(os.Args[2:])
	case "lookup":
		runLookup(os.Args[2:])
	case "add":
		runAdd(os.Args[2:])
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

func runServe() {
	cfg := config.Load()
	srv, err := server.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating server: %v\n", err)
		os.Exit(1)
	}
	if err := srv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
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

func runLookup(args []string) {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	isbn := fs.String("isbn", "", "ISBN to look up")
	title := fs.String("title", "", "book title")
	author := fs.String("author", "", "book author")
	fs.Parse(args)

	if *isbn == "" && *title == "" {
		fmt.Fprintln(os.Stderr, "usage: incipit lookup [--isbn X | --title T --author A]")
		os.Exit(2)
	}

	ctx := context.Background()

	var olResult, gbResult *models.LookupResult

	ol := lookup.NewOLClient("https://openlibrary.org")
	gb := lookup.NewGBClient("https://www.googleapis.com")

	if *isbn != "" {
		olResult, _ = ol.LookupByISBN(ctx, *isbn)
		gbResult, _ = gb.LookupByISBN(ctx, *isbn)
	} else {
		olResult, _ = ol.LookupByTitle(ctx, *title, *author)
		gbResult, _ = gb.LookupByTitle(ctx, *title, *author)
	}

	merged := lookup.Merge(olResult, gbResult)
	if merged == nil {
		fmt.Println("No results found")
		os.Exit(0)
	}

	fmt.Printf("Title:       %s\n", merged.Title)
	fmt.Printf("Author:      %s\n", merged.Author)
	if merged.Series != "" {
		fmt.Printf("Series:      %s\n", merged.Series)
	}
	if merged.Rating > 0 {
		fmt.Printf("Rating:      %.1f/5\n", merged.Rating)
	}
	if merged.Pages > 0 {
		fmt.Printf("Pages:       %d\n", merged.Pages)
	}
	if merged.Publisher != "" {
		fmt.Printf("Publisher:   %s\n", merged.Publisher)
	}
	if merged.Published != "" {
		fmt.Printf("Published:   %s\n", merged.Published)
	}
	if len(merged.Subjects) > 0 {
		fmt.Printf("Subjects:    %v\n", merged.Subjects)
	}
	if merged.Description != "" {
		fmt.Printf("Description: %s\n", merged.Description)
	}
	if merged.CoverURL != "" {
		fmt.Printf("Cover URL:   %s\n", merged.CoverURL)
	}
}

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	noLookup := fs.Bool("no-lookup", false, "skip metadata lookup")
	dryRun := fs.Bool("dry-run", false, "preview without saving")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: incipit add <path> [--no-lookup] [--dry-run]")
		os.Exit(2)
	}
	path := fs.Arg(0)

	fmt.Println("Parsing EPUB...")
	meta, err := epub.Parse(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing epub: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Title: %s\n", meta.Title)
	fmt.Printf("  Author: %s\n", meta.Creator)
	if meta.Identifier != "" {
		fmt.Printf("  ISBN: %s\n", meta.Identifier)
	}

	var lookupResult *models.LookupResult
	if !*noLookup {
		fmt.Println("\nLooking up metadata...")
		ctx := context.Background()
		ol := lookup.NewOLClient("https://openlibrary.org")
		gb := lookup.NewGBClient("https://www.googleapis.com")

		if meta.Identifier != "" {
			olResult, olErr := ol.LookupByISBN(ctx, meta.Identifier)
			if olErr != nil {
				fmt.Fprintf(os.Stderr, "  Open Library: %v\n", olErr)
			}
			gbResult, gbErr := gb.LookupByISBN(ctx, meta.Identifier)
			if gbErr != nil {
				fmt.Fprintf(os.Stderr, "  Google Books: %v\n", gbErr)
			}
			lookupResult = lookup.Merge(olResult, gbResult)
		} else if meta.Title != "" {
			olResult, olErr := ol.LookupByTitle(ctx, meta.Title, meta.Creator)
			if olErr != nil {
				fmt.Fprintf(os.Stderr, "  Open Library: %v\n", olErr)
			}
			gbResult, gbErr := gb.LookupByTitle(ctx, meta.Title, meta.Creator)
			if gbErr != nil {
				fmt.Fprintf(os.Stderr, "  Google Books: %v\n", gbErr)
			}
			lookupResult = lookup.Merge(olResult, gbResult)
		}

		if lookupResult != nil {
			fmt.Printf("  Found: %s by %s\n", lookupResult.Title, lookupResult.Author)
			if lookupResult.Series != "" {
				fmt.Printf("  Series: %s\n", lookupResult.Series)
			}
			if lookupResult.Rating > 0 {
				fmt.Printf("  Rating: %.1f/5\n", lookupResult.Rating)
			}
		} else {
			fmt.Println("  No lookup results found")
		}
	}

	book := models.MergeMetadata(meta, lookupResult)

	cfg := config.Load()
	s := storage.New(cfg.StorageDir)
	hash, err := s.HashFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error hashing file: %v\n", err)
		os.Exit(1)
	}
	book.FileHash = hash

	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting file info: %v\n", err)
		os.Exit(1)
	}
	book.FileSize = info.Size()

	if *dryRun {
		fmt.Println("\n[dry-run] Would add:")
		fmt.Printf("  Title: %s\n", book.Title)
		fmt.Printf("  Author: %s\n", book.Author)
		if book.Series != "" {
			fmt.Printf("  Series: %s\n", book.Series)
		}
		fmt.Printf("  ISBN: %s\n", book.ISBN)
		fmt.Printf("  File hash: %s\n", book.FileHash)
		fmt.Printf("  File size: %d bytes\n", book.FileSize)
		return
	}

	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()
	d.Migrate()

	book.FilePath = ""
	bookID, err := d.InsertBook(&book)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error inserting book: %v\n", err)
		os.Exit(1)
	}
	book.ID = bookID
	book.FilePath = fmt.Sprintf("files/%d.epub", bookID)
	d.UpdateBook(&book)

	if err := s.SaveBookFile(bookID, path); err != nil {
		fmt.Fprintf(os.Stderr, "error saving book file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n  Book ID: %d\n", bookID)
	fmt.Printf("  File: %s\n", s.BookFilePath(bookID))

	if lookupResult != nil && lookupResult.CoverURL != "" {
		resp, err := http.Get(lookupResult.CoverURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Cover download failed: %v\n", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				coverData, err := io.ReadAll(resp.Body)
				if err == nil {
					s.SaveCover(bookID, coverData)
					book.CoverPath = fmt.Sprintf("covers/%d.jpg", bookID)
					d.UpdateBook(&book)
					fmt.Printf("  Cover: %s\n", s.CoverPath(bookID))
				}
			}
		}
	}

	fmt.Printf("\nAdded: %s by %s", book.Title, book.Author)
	if book.Series != "" {
		fmt.Printf(" (%s)", book.Series)
	}
	fmt.Println()
}

var _ = strconv.Itoa
