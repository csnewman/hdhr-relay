package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

var (
	ErrInvalidPacket = errors.New("invalid packet")
	ErrEndOfPacket   = errors.New("end of packet")
)

func UnFrame(data []byte) (uint16, []byte, error) {
	if len(data) < 8 {
		return 0, nil, fmt.Errorf("%w: too short", ErrInvalidPacket)
	}

	ty := binary.BigEndian.Uint16(data[0:2])
	l := binary.BigEndian.Uint16(data[2:4])

	crcPos := l + 4

	if len(data) != int(crcPos)+4 {
		return 0, nil, fmt.Errorf("%w: invalid length", ErrInvalidPacket)
	}

	eCRC := binary.LittleEndian.Uint32(data[crcPos : crcPos+4])

	crc := crc32.ChecksumIEEE(data[0 : l+4])

	if crc != eCRC {
		return 0, nil, fmt.Errorf("%w: CRC mismatch", ErrInvalidPacket)
	}

	return ty, data[4 : l+4], nil
}

type Reader struct {
	Data []byte
	Pos  int
}

func NewReader(data []byte) *Reader {
	return &Reader{
		Data: data,
	}
}

func (r *Reader) ReadU8() (uint8, error) {
	if r.Pos+1 > len(r.Data) {
		return 0, ErrEndOfPacket
	}

	v := r.Data[r.Pos]
	r.Pos++

	return v, nil
}

func (r *Reader) ReadU32() (uint32, error) {
	if r.Pos+4 > len(r.Data) {
		return 0, ErrEndOfPacket
	}

	v := binary.BigEndian.Uint32(r.Data[r.Pos : r.Pos+4])
	r.Pos += 4

	return v, nil
}

func (r *Reader) ReadVarLen() (uint16, error) {
	if r.Pos+1 > len(r.Data) {
		return 0, ErrEndOfPacket
	}

	v := uint16(r.Data[r.Pos])
	r.Pos++

	if v&0x80 != 0 {
		if r.Pos+1 > len(r.Data) {
			return 0, ErrEndOfPacket
		}

		s := r.Data[r.Pos]
		r.Pos++

		v &= 0x7f
		v |= uint16(s) << 7
	}

	return v, nil
}

func (r *Reader) ReadStr(l int) (string, error) {
	if r.Pos+l > len(r.Data) {
		return "", ErrEndOfPacket
	}

	blob := r.Data[r.Pos : r.Pos+l]

	r.Pos += l

	return string(blob), nil
}

func (r *Reader) HasMore() bool {
	return r.Pos < len(r.Data)
}

func Frame(ty uint16, data []byte) []byte {
	out := make([]byte, len(data)+8)

	binary.BigEndian.PutUint16(out[0:2], ty)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(data)))

	w := copy(out[4:], data[:])

	if w != len(data) {
		panic("copy mismatch")
	}

	crcPos := len(data) + 4
	crc := crc32.ChecksumIEEE(out[0:crcPos])

	binary.LittleEndian.PutUint32(out[crcPos:crcPos+4], crc)

	return out
}

type Writer struct {
	Data *bytes.Buffer
}

func NewWriter() *Writer {
	return &Writer{
		Data: new(bytes.Buffer),
	}
}

func (w *Writer) WriteU8(v uint8) {
	w.Data.WriteByte(v)
}

func (w *Writer) WriteU32(v uint32) {
	w.Data.Write(binary.BigEndian.AppendUint32(nil, v))
}

func (w *Writer) WriteVarLen(v uint16) {
	if v <= 127 {
		w.WriteU8(uint8(v))

		return
	}

	w.WriteU8(uint8(v | 0x80))
	w.WriteU8(uint8(v >> 7))
}

func (w *Writer) WriteBlob(base []byte) {
	w.Data.Write(base)
}
