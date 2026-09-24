package service

import (
	"encoding/binary"
	"math/bits"
)

// sm3Sum computes the SM3 (GB/T 32905-2016) digest of data. It is used by the
// TEE task signer (Java Sm3Utils.hash / BouncyCastle SM3Digest).
func sm3Sum(data []byte) [32]byte {
	v := [8]uint32{
		0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600,
		0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e,
	}
	// padding: 0x80, zeros, 64-bit big-endian bit length.
	l := uint64(len(data)) * 8
	msg := make([]byte, 0, len(data)+72)
	msg = append(msg, data...)
	msg = append(msg, 0x80)
	for len(msg)%64 != 56 {
		msg = append(msg, 0)
	}
	var lb [8]byte
	binary.BigEndian.PutUint64(lb[:], l)
	msg = append(msg, lb[:]...)

	var w [68]uint32
	var w1 [64]uint32
	for off := 0; off < len(msg); off += 64 {
		for i := 0; i < 16; i++ {
			w[i] = binary.BigEndian.Uint32(msg[off+4*i:])
		}
		for i := 16; i < 68; i++ {
			x := w[i-16] ^ w[i-9] ^ bits.RotateLeft32(w[i-3], 15)
			w[i] = sm3P1(x) ^ bits.RotateLeft32(w[i-13], 7) ^ w[i-6]
		}
		for i := 0; i < 64; i++ {
			w1[i] = w[i] ^ w[i+4]
		}
		a, b, c, d, e, f, g, h := v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7]
		for j := 0; j < 64; j++ {
			tj := uint32(0x79cc4519)
			if j >= 16 {
				tj = 0x7a879d8a
			}
			a12 := bits.RotateLeft32(a, 12)
			ss1 := bits.RotateLeft32(a12+e+bits.RotateLeft32(tj, j%32), 7)
			ss2 := ss1 ^ a12
			var ff, gg uint32
			if j < 16 {
				ff = a ^ b ^ c
				gg = e ^ f ^ g
			} else {
				ff = (a & b) | (a & c) | (b & c)
				gg = (e & f) | (^e & g)
			}
			tt1 := ff + d + ss2 + w1[j]
			tt2 := gg + h + ss1 + w[j]
			d = c
			c = bits.RotateLeft32(b, 9)
			b = a
			a = tt1
			h = g
			g = bits.RotateLeft32(f, 19)
			f = e
			e = sm3P0(tt2)
		}
		v[0] ^= a
		v[1] ^= b
		v[2] ^= c
		v[3] ^= d
		v[4] ^= e
		v[5] ^= f
		v[6] ^= g
		v[7] ^= h
	}
	var out [32]byte
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[4*i:], v[i])
	}
	return out
}

func sm3P0(x uint32) uint32 { return x ^ bits.RotateLeft32(x, 9) ^ bits.RotateLeft32(x, 17) }
func sm3P1(x uint32) uint32 { return x ^ bits.RotateLeft32(x, 15) ^ bits.RotateLeft32(x, 23) }
