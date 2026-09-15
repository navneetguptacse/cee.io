package utils

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxZipEntries = 200
	MaxZipSize    = 20 * 1024 * 1024 // 20MB uncompressed max limit
)

// ExtractZipFromBase64 extracts a base64 encoded zip archive safely into destDir.
func ExtractZipFromBase64(b64Zip string, destDir string) error {
	zipBytes, err := base64.StdEncoding.DecodeString(b64Zip)
	if err != nil {
		return fmt.Errorf("invalid base64 zip data: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return fmt.Errorf("failed to parse zip archive: %w", err)
	}

	if len(reader.File) > MaxZipEntries {
		return fmt.Errorf("zip archive contains too many entries (%d > %d)", len(reader.File), MaxZipEntries)
	}

	var totalExtractedBytes int64
	cleanDestDir := filepath.Clean(destDir)

	for _, file := range reader.File {
		// Clean and validate path against Zip Slip
		cleanedPath := filepath.Clean(file.Name)
		if strings.HasPrefix(cleanedPath, "..") || filepath.IsAbs(cleanedPath) {
			return fmt.Errorf("zip archive contains dangerous path: %s", file.Name)
		}

		targetPath := filepath.Join(cleanDestDir, cleanedPath)
		if !strings.HasPrefix(targetPath, cleanDestDir+string(os.PathSeparator)) && targetPath != cleanDestDir {
			return fmt.Errorf("zip slip detected for entry: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		srcFile, err := file.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			srcFile.Close()
			return err
		}

		// Enforce decompression size limit (Zip Bomb protection)
		lr := io.LimitReader(srcFile, MaxZipSize-totalExtractedBytes+1)
		written, err := io.Copy(outFile, lr)
		srcFile.Close()
		outFile.Close()

		if err != nil {
			return fmt.Errorf("failed writing file %s: %w", file.Name, err)
		}

		totalExtractedBytes += written
		if totalExtractedBytes > MaxZipSize {
			return fmt.Errorf("zip archive exceeds maximum uncompressed size of %d bytes", MaxZipSize)
		}
	}

	return nil
}
