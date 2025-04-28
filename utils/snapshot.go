package utils

import (
	"fmt"
	"os"
)

const snapshotDir = "snapshot/"

func TakeSnapshot(dst string) error {
	file, err := os.Open(dst)
	if err != nil {
		return fmt.Errorf("failed to load leader commit data, %v", err)
	}
	defer file.Close()

	snapshot, err := os.OpenFile(snapshotDir+dst, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %v", err)
	}
	defer snapshot.Close()

	_, err = file.WriteTo(snapshot)
	if err != nil {
		return fmt.Errorf("failed to write file to snapshot: %v", err)
	}

	return nil
}

func RevertStates(dst string) error {
	snapshot, err := os.OpenFile(snapshotDir+dst, os.O_CREATE|os.O_RDWR, 0666)
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
