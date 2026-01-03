package storage

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
)

type JsonDb[T any] struct {
	mux      sync.Mutex
	value    T
	filePath string
}

func (v *JsonDb[T]) Save() error {
	v.mux.Lock()
	defer v.mux.Unlock()

	b, err := json.MarshalIndent(v.value, "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(v.filePath, b, 0644)
}

func (v *JsonDb[T]) Read() error {
	if _, err := os.Stat(v.filePath); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	f, err := os.Open(v.filePath)
	if err != nil {
		return err
	}

	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, &v.value)
}

func (v *JsonDb[T]) Update(fn func(v T) T) {
	v.value = fn(v.value)
	_ = v.Save()
}

func (v *JsonDb[T]) Set(value *T) {
	v.mux.Lock()
	v.value = *value
	v.mux.Unlock()
	_ = v.Save()
}

func (v *JsonDb[T]) Get() *T {
	return &v.value
}

func CreateJsonDB[T any](filePath string, initialValue T) *JsonDb[T] {
	return &JsonDb[T]{filePath: filePath, value: initialValue, mux: sync.Mutex{}}
}
