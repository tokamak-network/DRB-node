package utils

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

const snapshotDir = "./snapshots/"

func TakeSnapshot(dst string) {
	log.Printf("Taking snapshot for %s", dst)

	file, err := os.Open(dst)
	if err != nil {
		log.Printf("failed to load leader commit data, %v", err)
	}
	defer file.Close()

	// Create the snapshot directory if it doesn't exist
	if err := os.MkdirAll(snapshotDir, os.ModePerm); err != nil {
		log.Printf("failed to create snapshot directory: %v", err)
		return
	}

	// Create the snapshot file path
	filepath := filepath.Join(snapshotDir, dst)

	// Open or create the snapshot file
	snapshot, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Printf("failed to open/create snapshot file: %v", err)
		return
	}
	defer snapshot.Close()

	_, err = file.WriteTo(snapshot)
	if err != nil {
		log.Printf("failed to write file to snapshot: %v", err)
		return
	}

	log.Printf("Succeed to take snapshot of %s", dst)
}

func RevertStates(dst string) {
	log.Printf("Reverting states from %s", dst)

	filepath := filepath.Join(snapshotDir, dst)

	snapshot, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("failed to open snapshot file: %v", err)
		return
	}
	defer snapshot.Close()

	file, err := os.Open(dst)
	if err != nil {
		log.Printf("failed to open leader commit file, %v", err)
		return
	}
	defer file.Close()

	_, err = io.Copy(snapshot, file)
	if err != nil {
		log.Printf("failed to revert states from snapshot: %v", err)
		return
	}

	log.Printf("%s is reverted successfully", dst)
}
