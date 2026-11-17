// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"github.com/remzone/blink_windows/internal/client"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"unsafe"
)

type openFileName struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File                         *uint16
	MaxFile                      uint32
	FileTitle                    *uint16
	MaxFileTitle                 uint32
	InitialDir, Title            *uint16
	Flags                        uint32
	FileOffset, FileExtension    uint16
	DefExt                       *uint16
	CustData                     uintptr
	Hook                         uintptr
	TemplateName                 *uint16
	Reserved                     uintptr
	Reserved2, FlagsEx           uint32
}

func (a *app) importFile() {
	if a.tunnel != nil || a.busy {
		return
	}
	buffer := make([]uint16, 32768)
	filter := append(windows.StringToUTF16("Конфигурации (*.conf;*.json;*.txt;*.yaml)"), windows.StringToUTF16("*.conf;*.json;*.txt;*.yaml")...)
	filter = append(filter, windows.StringToUTF16("Все файлы")...)
	filter = append(filter, windows.StringToUTF16("*.*")...)
	filter = append(filter, 0)
	of := openFileName{Size: uint32(unsafe.Sizeof(openFileName{})), Owner: a.hwnd, Filter: &filter[0], File: &buffer[0], MaxFile: uint32(len(buffer)), Flags: 0x1000 | 0x80000 | 0x8}
	ok, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&of)))
	if ok == 0 {
		return
	}
	f, err := os.Open(windows.UTF16ToString(buffer))
	if err != nil {
		a.notice("Не удалось открыть файл")
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, client.MaxDocument+1))
	if err != nil || len(b) > client.MaxDocument {
		a.notice("Файл не прочитан или превышает 1 МиБ")
		return
	}
	a.importSource(string(b), "")
}
