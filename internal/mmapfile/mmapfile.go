package mmapfile

import (
	"os"
	"syscall"
)

func Open(path string) (data []byte, close func() error, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}

	if fi.Size() == 0 {
		return []byte{}, func() error { return nil }, nil
	}

	b, err := syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}

	return b, func() error { return syscall.Munmap(b) }, nil
}
