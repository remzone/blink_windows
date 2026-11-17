// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "Blink")
	err = os.MkdirAll(dir, 0700)
	return dir, err
}
func protect(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty protected data")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, errors.New("Windows DPAPI: не удалось открыть или сохранить профили для текущего пользователя")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func LoadState() ([]byte, error) {
	dir, err := DataDir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "profiles.dpapi"))
	if err != nil {
		return nil, err
	}
	return protect(b, true)
}
func SaveState(data []byte) error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	b, err := protect(data, false)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "profiles-*.dpapi")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return windows.MoveFileEx(windows.StringToUTF16Ptr(name), windows.StringToUTF16Ptr(filepath.Join(dir, "profiles.dpapi")), windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
