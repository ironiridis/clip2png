package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	dllUser32   = syscall.MustLoadDLL("user32")
	dllKernel32 = syscall.MustLoadDLL("kernel32")

	// https://docs.microsoft.com/en-us/windows/desktop/dataxchg/clipboard-functions
	cbOpen    = dllUser32.MustFindProc("OpenClipboard")
	cbClose   = dllUser32.MustFindProc("CloseClipboard")
	cbEmpty   = dllUser32.MustFindProc("EmptyClipboard")
	cbGetData = dllUser32.MustFindProc("GetClipboardData")

	// https://docs.microsoft.com/en-us/windows/desktop/api/winbase/nf-winbase-globallock
	hndGet = dllKernel32.MustFindProc("GlobalLock")
	hndPut = dllKernel32.MustFindProc("GlobalUnlock")
)

// Error is used for providing comparable error strings, not that we'll ever do that
// https://dave.cheney.net/2016/04/07/constant-errors
type Error string

func (e Error) Error() string { return string(e) }

// https://docs.microsoft.com/en-us/windows/desktop/dataxchg/standard-clipboard-formats
const fmtDIB = 8 // CF_DIB

func tryOpenClipboard() error {
	r, _, err := cbOpen.Call(0)
	if r != 0 {
		return nil
	}
	return err
}

func readClipboard(img *image.NRGBA) error {
	if err := tryOpenClipboard(); err != nil {
		return err
	}
	defer cbClose.Call()
	handle, _, err := cbGetData.Call(fmtDIB)
	if handle == 0 {
		return err
	}

	ptr, _, err := hndGet.Call(handle)
	if ptr == 0 {
		return err
	}
	defer hndPut.Call(handle)
	defer cbEmpty.Call()

	type bitmapHeader struct {
		biSize          uint32
		biWidth         int32
		biHeight        int32
		biPlanes        uint16
		biBitCount      uint16
		biCompression   uint32
		biSizeImage     uint32
		biXPelsPerMeter int32
		biYPelsPerMeter int32
		biClrUsed       uint32
		biClrImportant  uint32
	}
	hdr := (*bitmapHeader)(unsafe.Pointer(ptr))
	src := (*[1 << 48]uint8)(unsafe.Pointer(ptr + uintptr(hdr.biSize)))
	switch hdr.biBitCount {
	case 32:
		for y := (img.Rect.Dy() - 1); y >= 0; y-- {
			for x := (img.Rect.Dx() - 1); x >= 0; x-- {
				o := ((int(hdr.biHeight-1) - y) * int(hdr.biWidth) * 4) + (x * 4)
				img.SetNRGBA(x, y, color.NRGBA{B: src[o+0], G: src[o+1], R: src[o+2], A: src[o+3]})
			}
		}
	case 24:
		// 3*hdr.biWidth = the number of bytes of actual image data in the buffer
		// however, all lines must be a multiple of 4 bytes according to the docs
		// so this math gives us the number of extra bytes on each line
		//linePad := (4 - (3*hdr.biWidth)%4) % 4

		return Error("unimplemented, sorry...")
	default:
		return Error("unsuported bit depth")
	}
	return nil
}

func main() {
	flagW := flag.Int("w", 1920, "width of output image")
	flagH := flag.Int("h", 1080, "height of output image")
	flagI := flag.Int("i", 500, "index to start capture")
	flagF := flag.String("f", "cap-%06d.png", "output filename format")
	flag.Parse()
	img := image.NewNRGBA(image.Rect(0, 0, *flagW, *flagH))
	fmt.Printf("created an output image of dimension %v\n", img.Rect.Size())

	idx := *flagI
	for {
		err := readClipboard(img)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		fn := fmt.Sprintf(*flagF, idx)
		f, err := os.Create(fn)
		if err != nil {
			panic(err)
		}

		if err := png.Encode(f, img); err != nil {
			f.Close()
			panic(err)
		}

		if err := f.Close(); err != nil {
			panic(err)
		}
		fmt.Printf("wrote %s\n", fn)
		idx++
	}
}
