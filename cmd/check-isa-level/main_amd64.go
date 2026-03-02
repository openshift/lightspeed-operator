//go:build linux && amd64

package main

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	ntGNUPropertyType0       = 5
	gnuPropertyX86ISA1Needed = 0xc0008002
	isaV3                    = 1 << 2
	isaV4                    = 1 << 3
	isaBeyondV2              = isaV3 | isaV4
)

func main() {
	path := "manager"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "binary not found: %s\n", path)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "stat %s: %v\n", path, err)
		os.Exit(2)
	}
	ok, err := atMostX86_64_V2(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading ELF %s: %v\n", path, err)
		os.Exit(2)
	}
	if !ok {
		val, _, _ := getX86ISA1Needed(path)
		fmt.Fprintf(os.Stderr, "%s requires ISA level beyond x86-64-v2 (GNU_PROPERTY_X86_ISA_1_NEEDED=0x%x has v3/v4 bits)\n", path, val)
		os.Exit(1)
	}
	os.Exit(0)
}

func getX86ISA1Needed(path string) (uint32, bool, error) {
	f, err := elf.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()

	var noteSection *elf.Section
	for _, s := range f.Sections {
		if s.Name == ".note.gnu.property" {
			noteSection = s
			break
		}
	}
	if noteSection == nil {
		for _, s := range f.Sections {
			if s.Type == elf.SHT_NOTE {
				noteSection = s
				break
			}
		}
	}
	if noteSection == nil {
		return 0, false, nil
	}

	r := noteSection.Open()
	data := make([]byte, noteSection.Size)
	if _, err := r.Read(data); err != nil {
		return 0, false, err
	}

	order := binary.LittleEndian
	off := 0
	for off+12 <= len(data) {
		namesz := order.Uint32(data[off : off+4])
		descsz := order.Uint32(data[off+4 : off+8])
		ntype := order.Uint32(data[off+8 : off+12])
		off += 12

		namePadded := (namesz + 3) &^ 3
		if off+int(namePadded)+int(descsz) > len(data) {
			break
		}
		name := data[off : off+int(namesz)]
		off += int(namePadded)
		desc := data[off : off+int(descsz)]
		off += int((descsz + 3) &^ 3)

		if ntype != ntGNUPropertyType0 {
			continue
		}
		if namesz < 4 || string(name[:4]) != "GNU\x00" {
			continue
		}

		descOff := 0
		for descOff+8 <= len(desc) {
			prType := order.Uint32(desc[descOff : descOff+4])
			prDatasz := order.Uint32(desc[descOff+4 : descOff+8])
			descOff += 8
			if descOff+int(prDatasz) > len(desc) {
				break
			}
			if prType == gnuPropertyX86ISA1Needed && prDatasz == 4 {
				return order.Uint32(desc[descOff : descOff+4]), true, nil
			}
			descOff += int((prDatasz + 7) &^ 7)
		}
	}

	return 0, false, nil
}

func atMostX86_64_V2(path string) (bool, error) {
	val, found, err := getX86ISA1Needed(path)
	if err != nil || !found {
		return false, err
	}
	return (val & isaBeyondV2) == 0, nil
}
