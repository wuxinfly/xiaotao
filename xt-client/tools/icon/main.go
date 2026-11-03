// Generate the executable's multi-resolution ICO using the desktop renderer.
package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"xiaotao/xt-client/internal/iconimage"
)

func main() {
	if err := generate(); err != nil {
		panic(err)
	}
}

func generate() error {
	f, err := os.Open("winres/icon.png")
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	header := make([]byte, 6+16*len(sizes))
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], uint16(len(sizes)))
	var payload bytes.Buffer
	for i, size := range sizes {
		var frame bytes.Buffer
		if err := png.Encode(&frame, iconimage.Render(src, size)); err != nil {
			return err
		}
		entry := header[6+i*16:]
		entry[0], entry[1] = byte(size%256), byte(size%256)
		binary.LittleEndian.PutUint16(entry[4:], 1)
		binary.LittleEndian.PutUint16(entry[6:], 32)
		binary.LittleEndian.PutUint32(entry[8:], uint32(frame.Len()))
		binary.LittleEndian.PutUint32(entry[12:], uint32(len(header)+payload.Len()))
		payload.Write(frame.Bytes())
	}
	if err := os.MkdirAll("dist/icon", 0755); err != nil {
		return err
	}
	return os.WriteFile("dist/icon/app.ico", append(header, payload.Bytes()...), 0644)
}
