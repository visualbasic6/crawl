package storage

import (
    "fmt"
    "os"
    "sync"
)

type FileStorage struct {
    mu       sync.Mutex
    filename string
}

func NewFileStorage(filename string) *FileStorage {
    return &FileStorage{
        filename: filename,
    }
}

func (f *FileStorage) AppendNode(nodeURL string) error {
    f.mu.Lock()
    defer f.mu.Unlock()

    file, err := os.OpenFile(f.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open file: %w", err)
    }
    defer file.Close()

    if _, err := file.WriteString(nodeURL + "\n"); err != nil {
        return fmt.Errorf("failed to write to file: %w", err)
    }

    return nil
}
