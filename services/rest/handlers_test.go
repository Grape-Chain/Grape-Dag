package rest

import (
	"errors"
	"testing"
)

func TestDerefOrReturnsDefaultForNil(t *testing.T) {
	var nilInt *int
	if got := derefOr(nilInt, defaultPageSize); got != defaultPageSize {
		t.Fatalf("derefOr(nil) = %d, want %d", got, defaultPageSize)
	}

	set := 42
	if got := derefOr(&set, defaultPageSize); got != 42 {
		t.Fatalf("derefOr(&42) = %d, want 42", got)
	}

	var nilSlice *[]string
	if got := derefOr(nilSlice, []string{}); got == nil || len(got) != 0 {
		t.Fatalf("derefOr(nil slice ptr) = %v, want an empty slice", got)
	}
}

// The documented defaults in api/openapi.yml are not generated into the server,
// so the handlers supply them. Keep the two in step.
func TestParamDefaultsMatchTheSpec(t *testing.T) {
	if defaultPage != 0 {
		t.Errorf("defaultPage = %d, spec says 0", defaultPage)
	}
	if defaultPageSize != 15 {
		t.Errorf("defaultPageSize = %d, spec says 15", defaultPageSize)
	}
	if defaultSortOrder != "DESC" {
		t.Errorf("defaultSortOrder = %q, spec says DESC", defaultSortOrder)
	}
}

// ErrorRecovery used to type-assert the recovered value to error, which panicked
// again for the handlers that panic with a plain string - escaping the middleware
// and dropping the connection with no response.
func TestPanicMessageHandlesEveryPanicValue(t *testing.T) {
	cases := []struct {
		name string
		val  interface{}
		want string
	}{
		{"error", errors.New("boom"), "boom"},
		{"string", "Bad ABI json", "Bad ABI json"},
		{"other", 42, "42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := func() (msg string) {
				defer func() {
					if r := recover(); r != nil {
						msg = panicMessage(r)
					}
				}()
				panic(tc.val)
			}()
			if got != tc.want {
				t.Fatalf("panicMessage(%v) = %q, want %q", tc.val, got, tc.want)
			}
		})
	}
}

func TestAuthCredentialsConfiguredRejectsEmptyPair(t *testing.T) {
	// config.REST_API_USERNAME/PASSWORD are read from the environment at init;
	// in the test binary they are empty, which is exactly the case that used to
	// make `Authorization: Basic Og==` a valid credential for every route.
	if authCredentialsConfigured() {
		t.Skip("API credentials are set in this environment")
	}
	if authCredentialsConfigured() {
		t.Fatalf("empty credentials must not count as configured")
	}
}
