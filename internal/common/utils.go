package common

import (
	"fmt"
	"io"
	"os"
	"unsafe"
)

// LoadBucket returns an InBucket viewing buf in place.
//
// The on-disk value of a leaf element starts at pos+ksize, so an odd-length key
// can place it on an address that is not 8-byte aligned, and InBucket is two
// uint64s. That is a misaligned 64-bit access: fine on amd64, but it faults
// with SIGBUS on architectures that require alignment.
//
// NOTE: the result deliberately aliases buf. Callers such as the surgeon mutate
// the returned struct and then write buf back, so decoding into a fresh struct
// would silently discard those edits. Anything that only reads should not rely
// on that aliasing; anything that writes must go through buf.
func LoadBucket(buf []byte) *InBucket {
	if len(buf) < BucketHeaderSize {
		return &InBucket{}
	}
	return (*InBucket)(unsafe.Pointer(&buf[0]))
}

// LoadPage returns a Page viewing buf in place; see LoadBucket for why the
// result aliases buf.
func LoadPage(buf []byte) *Page {
	if len(buf) < int(PageHeaderSize) {
		return &Page{}
	}
	return (*Page)(unsafe.Pointer(&buf[0]))
}

// LoadPageMeta returns a Meta viewing buf in place, starting after the page
// header; see LoadBucket for why the result aliases buf.
func LoadPageMeta(buf []byte) *Meta {
	if len(buf) < int(PageHeaderSize)+int(unsafe.Sizeof(Meta{})) {
		return &Meta{}
	}
	return (*Meta)(unsafe.Pointer(&buf[PageHeaderSize]))
}

func CopyFile(srcPath, dstPath string) error {
	// Ensure source file exists.
	_, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("source file %q not found", srcPath)
	} else if err != nil {
		return err
	}

	// Ensure output file not exist.
	_, err = os.Stat(dstPath)
	if err == nil {
		return fmt.Errorf("output file %q already exists", dstPath)
	} else if !os.IsNotExist(err) {
		return err
	}

	srcDB, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %q: %w", srcPath, err)
	}
	defer srcDB.Close()
	dstDB, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %q: %w", dstPath, err)
	}
	defer dstDB.Close()
	written, err := io.Copy(dstDB, srcDB)
	if err != nil {
		return fmt.Errorf("failed to copy database file from %q to %q: %w", srcPath, dstPath, err)
	}

	srcFi, err := srcDB.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info %q: %w", srcPath, err)
	}
	initialSize := srcFi.Size()
	if initialSize != written {
		return fmt.Errorf("the byte copied (%q: %d) isn't equal to the initial db size (%q: %d)", dstPath, written, srcPath, initialSize)
	}

	return nil
}
