package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/configure"
)

const testSecret = "test-secret"

func signToken(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	require.NoError(t, err)
	return s
}

func TestJWTMiddleware(t *testing.T) {
	configure.Config.Set("jwt.secret", testSecret)
	configure.Config.Set("jwt.algorithm", "HS256")
	t.Cleanup(func() {
		configure.Config.Set("jwt.secret", "")
		configure.Config.Set("jwt.algorithm", "")
	})

	valid := signToken(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{"sub": "test"})
	expired := signToken(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	wrongSecret := signToken(t, jwt.SigningMethodHS256, []byte("other"), jwt.MapClaims{})
	wrongAlg := signToken(t, jwt.SigningMethodHS512, []byte(testSecret), jwt.MapClaims{})
	none := signToken(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, jwt.MapClaims{})

	tests := []struct {
		name   string
		method string
		url    string
		header string
		want   int
	}{
		{name: "bearer header", url: "/", header: "Bearer " + valid, want: http.StatusOK},
		{name: "lowercase bearer", url: "/", header: "bearer " + valid, want: http.StatusOK},
		{name: "query parameter", url: "/?jwt=" + valid, want: http.StatusOK},
		{name: "options skips auth", method: http.MethodOptions, url: "/", want: http.StatusOK},
		{name: "missing token", url: "/", want: http.StatusForbidden},
		{name: "malformed header", url: "/", header: valid, want: http.StatusForbidden},
		{name: "header takes precedence", url: "/?jwt=" + valid, header: "Basic abc", want: http.StatusForbidden},
		{name: "expired", url: "/", header: "Bearer " + expired, want: http.StatusForbidden},
		{name: "wrong secret", url: "/", header: "Bearer " + wrongSecret, want: http.StatusForbidden},
		{name: "wrong algorithm", url: "/", header: "Bearer " + wrongAlg, want: http.StatusForbidden},
		{name: "none algorithm", url: "/", header: "Bearer " + none, want: http.StatusForbidden},
	}

	handler := JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, tt.url, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

func TestJWTMiddlewareDisabledWithoutSecret(t *testing.T) {
	configure.Config.Set("jwt.secret", "")

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()

	JWTMiddleware(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
}
