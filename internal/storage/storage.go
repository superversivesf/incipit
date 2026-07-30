package storage

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Storage struct {
	rootDir string
}

func New(rootDir string) *Storage {
	return &Storage{rootDir: rootDir}
}

func (s *Storage) SaveBookFile(bookID int64, sourcePath string) error {
	dst := s.BookFilePath(bookID)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating files dir: %w", err)
	}
	return copyFile(sourcePath, dst)
}

func (s *Storage) SaveCover(bookID int64, imageData []byte) error {
	dst := s.CoverPath(bookID)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating covers dir: %w", err)
	}
	return os.WriteFile(dst, imageData, 0644)
}

func (s *Storage) HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hashing: %w", err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Storage) BookFilePath(bookID int64) string {
	return filepath.Join(s.rootDir, "files", fmt.Sprintf("%d.epub", bookID))
}

func (s *Storage) CoverPath(bookID int64) string {
	return filepath.Join(s.rootDir, "covers", fmt.Sprintf("%d.jpg", bookID))
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}
	return nil
}
