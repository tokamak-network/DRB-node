package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const snapshotDir = "./snapshots/"

func TakeSnapshot(dst string) error {
	file, err := os.Open(dst)
	if err != nil {
		log.Printf("failed to load leader commit data, %v", err)
	}
	defer file.Close()

	// Create the snapshot directory if it doesn't exist
	if err := os.MkdirAll(snapshotDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %v", err)
	}

	// Create the snapshot file path
	filepath := filepath.Join(snapshotDir, dst)

	// Open or create the snapshot file
	snapshot, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("failed to open/create snapshot file: %v", err)
	}
	defer snapshot.Close()

	_, err = file.WriteTo(snapshot)
	if err != nil {
		return fmt.Errorf("failed to write file to snapshot: %v", err)
	}

	return nil
}

func RevertStates(dst string) error {
	filepath := filepath.Join(snapshotDir, dst)

	snapshot, err := os.OpenFile(filepath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %v", err)
	}
	defer snapshot.Close()

	file, err := os.Open(dst)
	if err != nil {
		return fmt.Errorf("failed to open leader commit file, %v", err)
	}
	defer file.Close()

	_, err = snapshot.WriteTo(file)
	if err != nil {
		return fmt.Errorf("failed to revert states from snapshot: %v", err)
	}

	return nil
}
