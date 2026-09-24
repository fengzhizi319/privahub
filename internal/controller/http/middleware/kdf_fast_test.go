package middleware

import "github.com/fengzhizi319/privahub/pkg/auth"

// Unit tests use a tiny SM3 KDF work factor (production: 1,000,000).
func init() { auth.SetPasswordKDFIterationsForTesting(16) }
