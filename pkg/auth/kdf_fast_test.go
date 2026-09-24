package auth

// Unit tests use a tiny SM3 KDF work factor (production: 1,000,000);
// TestPasswordVerify_MinTimeBudget measures the production default directly.
func init() { SetPasswordKDFIterationsForTesting(16) }
