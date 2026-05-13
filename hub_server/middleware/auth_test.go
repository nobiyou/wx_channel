package middleware

import "testing"

func TestInitJWTSecretFromEnv(t *testing.T) {
	t.Run("missing env", func(t *testing.T) {
		oldSecret := JWTSecret
		defer func() { JWTSecret = oldSecret }()

		t.Setenv("HUB_JWT_SECRET", "")
		JWTSecret = nil
		if err := InitJWTSecretFromEnv(); err != nil {
			t.Fatalf("unexpected error for missing HUB_JWT_SECRET: %v", err)
		}
		if len(JWTSecret) != 32 {
			t.Fatalf("expected generated JWTSecret length 32, got %d", len(JWTSecret))
		}
	})

	t.Run("too short", func(t *testing.T) {
		t.Setenv("HUB_JWT_SECRET", "short-secret")
		if err := InitJWTSecretFromEnv(); err == nil {
			t.Fatalf("expected error for short secret")
		}
	})

	t.Run("valid secret", func(t *testing.T) {
		oldSecret := JWTSecret
		defer func() { JWTSecret = oldSecret }()

		secret := "this-is-a-very-strong-secret-with-32+chars"
		t.Setenv("HUB_JWT_SECRET", secret)
		if err := InitJWTSecretFromEnv(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(JWTSecret) != secret {
			t.Fatalf("JWTSecret not updated")
		}
	})

	t.Run("nil before init", func(t *testing.T) {
		oldSecret := JWTSecret
		defer func() { JWTSecret = oldSecret }()

		JWTSecret = nil
		if JWTSecret != nil {
			t.Fatal("JWTSecret should be nil before initialization")
		}
	})
}
