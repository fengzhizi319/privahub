package service

import "github.com/fengzhizi319/privahub/pkg/sm3"

// sm3Sum computes the SM3 (GB/T 32905-2016) digest of data. It is used by the
// TEE task signer (Java Sm3Utils.hash / BouncyCastle SM3Digest). The
// implementation lives in pkg/sm3 (shared with the audit hash chain).
func sm3Sum(data []byte) [32]byte { return sm3.Sum(data) }
