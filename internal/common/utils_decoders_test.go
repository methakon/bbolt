package common

import (
	"math/rand"
	"testing"
	"unsafe"
)

// The loaders return views onto buf, not copies. That contract is load-bearing:
// internal/surgeon mutates the returned Meta and then writes buf back, so a
// copy-returning implementation silently discards those edits. This pins the
// behaviour so a future "clean it up" refactor cannot break it unnoticed.
func TestLoadersAliasBuffer(t *testing.T) {
	t.Run("InBucket", func(t *testing.T) {
		buf := make([]byte, BucketHeaderSize)
		ib := LoadBucket(buf)
		ib.SetRootPage(42)
		ib.SetInSequence(99)
		if v := (*InBucket)(unsafe.Pointer(&buf[0])); v.RootPage() != 42 || v.InSequence() != 99 {
			t.Fatalf("LoadBucket does not alias buf: root=%d seq=%d",
				v.RootPage(), v.InSequence())
		}
	})

	t.Run("Page", func(t *testing.T) {
		buf := make([]byte, int(PageHeaderSize))
		p := LoadPage(buf)
		p.SetFlags(LeafPageFlag)
		p.SetCount(3)
		if v := (*Page)(unsafe.Pointer(&buf[0])); !v.IsLeafPage() || v.Count() != 3 {
			t.Fatalf("LoadPage does not alias buf: leaf=%v count=%d", v.IsLeafPage(), v.Count())
		}
	})

	t.Run("Meta", func(t *testing.T) {
		buf := make([]byte, int(PageHeaderSize)+int(unsafe.Sizeof(Meta{})))
		m := LoadPageMeta(buf)
		m.SetFreelist(PgidNoFreelist)
		v := (*Meta)(unsafe.Pointer(&buf[PageHeaderSize]))
		if v.Freelist() != PgidNoFreelist {
			t.Fatalf("LoadPageMeta does not alias buf: freelist=%d", v.Freelist())
		}
	})
}

func TestLoadersMatchUnsafeCasts(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	// The prior implementations, reproduced verbatim, so the comparison is
	// against real previous behaviour rather than a description of it.
	oldLoadBucket := func(b []byte) *InBucket { return (*InBucket)(unsafe.Pointer(&b[0])) }
	oldLoadPage := func(b []byte) *Page { return (*Page)(unsafe.Pointer(&b[0])) }
	oldLoadPageMeta := func(b []byte) *Meta {
		return (*Meta)(unsafe.Pointer(&b[PageHeaderSize]))
	}

	t.Run("InBucket", func(t *testing.T) {
		for i := 0; i < 2000; i++ {
			// Vary the alignment deliberately: odd offsets are the whole point.
			buf := make([]byte, BucketHeaderSize+8)
			off := rng.Intn(8)
			_, _ = rng.Read(buf)
			b, old := LoadBucket(buf[off:]), oldLoadBucket(buf[off:])
			if b.RootPage() != old.RootPage() || b.InSequence() != old.InSequence() {
				t.Fatalf("off=%d new=(%d,%d) old=(%d,%d)", off,
					b.RootPage(), b.InSequence(), old.RootPage(), old.InSequence())
			}
		}
	})

	t.Run("Page", func(t *testing.T) {
		for i := 0; i < 2000; i++ {
			buf := make([]byte, int(PageHeaderSize)+8)
			off := rng.Intn(8)
			_, _ = rng.Read(buf)
			p, old := LoadPage(buf[off:]), oldLoadPage(buf[off:])
			if p.Id() != old.Id() || p.Typ() != old.Typ() ||
				p.Count() != old.Count() || p.Overflow() != old.Overflow() ||
				p.IsLeafPage() != old.IsLeafPage() || p.IsBranchPage() != old.IsBranchPage() ||
				p.IsMetaPage() != old.IsMetaPage() {
				t.Fatalf("off=%d mismatch: new=(%d,%s,%d) old=(%d,%s,%d)", off,
					p.Id(), p.Typ(), p.Count(), old.Id(), old.Typ(), old.Count())
			}
		}
	})

	t.Run("Meta", func(t *testing.T) {
		for i := 0; i < 2000; i++ {
			buf := make([]byte, int(PageHeaderSize)+int(unsafe.Sizeof(Meta{}))+8)
			off := rng.Intn(8)
			_, _ = rng.Read(buf)
			m, old := LoadPageMeta(buf[off:]), oldLoadPageMeta(buf[off:])
			if m.Magic() != old.Magic() || m.Version() != old.Version() ||
				m.PageSize() != old.PageSize() || m.Flags() != old.Flags() ||
				m.RootBucket().RootPage() != old.RootBucket().RootPage() ||
				m.RootBucket().InSequence() != old.RootBucket().InSequence() ||
				m.Freelist() != old.Freelist() ||
				m.Pgid() != old.Pgid() || m.Txid() != old.Txid() ||
				m.Checksum() != old.Checksum() {
				t.Fatalf("off=%d mismatch: new magic=%v old magic=%v", off, m.Magic(), old.Magic())
			}
		}
	})
}

// A short buffer must not read past the end and must not panic.
func TestLoadersRejectShortBuffers(t *testing.T) {
	for n := 0; n < int(PageHeaderSize)+int(unsafe.Sizeof(Meta{})); n++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("len=%d panicked: %v", n, r)
				}
			}()
			buf := make([]byte, n)
			LoadBucket(buf)
			LoadPage(buf)
			LoadPageMeta(buf)
		}()
	}
}

// The real-page alignment case from the report: an odd-length key puts the
// InBucket at an unaligned offset, and the value must still decode correctly.
func TestUnalignedLeafBucketValueRoundTrips(t *testing.T) {
	for _, keyLen := range []int{1, 2, 3, 5, 7, 8, 9} {
		buf := make([]byte, BucketHeaderSize+keyLen)
		off := keyLen // value begins at pos+ksize
		buf[off] = 7
		buf[off+8] = 99

		base := uintptr(unsafe.Pointer(&buf[off]))
		got := LoadBucket(buf[off:])
		if got.RootPage() != 7 || got.InSequence() != 99 {
			t.Fatalf("keyLen=%d got root=%d seq=%d", keyLen, got.RootPage(), got.InSequence())
		}
		t.Logf("keyLen=%d value addr mod 8 = %d -> root=%d seq=%d",
			keyLen, base%8, got.RootPage(), got.InSequence())
	}
}
