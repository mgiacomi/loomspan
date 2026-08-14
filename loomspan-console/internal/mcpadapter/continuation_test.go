package mcpadapter

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestContinuationRoundTripForEveryOperationKind(t *testing.T) {
	for _, test := range []struct {
		kind      continuationKind
		sessionID string
	}{{continuationSkills, ""}, {continuationExecutions, ""}, {continuationActivity, "session-1"}} {
		token, err := encodeContinuation(test.kind, "scope-1", "cursor-1", test.sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(token, "=") {
			t.Fatalf("continuation is padded: %q", token)
		}
		cursor, domain := decodeContinuation(token, test.kind, "scope-1", test.sessionID)
		if domain != nil || cursor != "cursor-1" {
			t.Fatalf("kind=%s cursor=%q domain=%#v", test.kind, cursor, domain)
		}
	}
}

func TestContinuationRejectsMalformedAndMismatchedInput(t *testing.T) {
	valid, err := encodeContinuation(continuationActivity, "scope-1", "cursor-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	unknown := base64.RawURLEncoding.EncodeToString([]byte(`{"version":1,"kind":"activity","targetScopeId":"scope-1","cursor":"cursor-1","sessionId":"session-1","extra":true}`))
	trailing := base64.RawURLEncoding.EncodeToString([]byte(`{"version":1,"kind":"activity","targetScopeId":"scope-1","cursor":"cursor-1","sessionId":"session-1"}{}`))
	badVersion := base64.RawURLEncoding.EncodeToString([]byte(`{"version":2,"kind":"activity","targetScopeId":"scope-1","cursor":"cursor-1","sessionId":"session-1"}`))
	for name, token := range map[string]string{
		"empty": "", "padding": valid + "=", "alphabet": "***", "unknown": unknown,
		"trailing": trailing, "version": badVersion,
	} {
		t.Run(name, func(t *testing.T) {
			if _, domain := decodeContinuation(token, continuationActivity, "scope-1", "session-1"); domain == nil || domain.Code != "INVALID_ARGUMENT" {
				t.Fatalf("domain = %#v", domain)
			}
		})
	}
	if _, domain := decodeContinuation(valid, continuationSkills, "scope-1", ""); domain == nil || domain.Code != "INVALID_ARGUMENT" {
		t.Fatalf("cross-operation domain = %#v", domain)
	}
	if _, domain := decodeContinuation(valid, continuationActivity, "scope-1", "session-2"); domain == nil || domain.Code != "INVALID_ARGUMENT" {
		t.Fatalf("cross-session domain = %#v", domain)
	}
}

func TestContinuationReturnsTargetChangedForPriorScope(t *testing.T) {
	token, err := encodeContinuation(continuationSkills, "scope-old", "cursor-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, domain := decodeContinuation(token, continuationSkills, "scope-new", ""); domain == nil || domain.Code != "TARGET_CHANGED" || domain.TargetScopeID != "scope-old" {
		t.Fatalf("domain = %#v", domain)
	}
}

func TestContinuationBounds(t *testing.T) {
	token, err := encodeContinuation(continuationSkills, "scope-1", strings.Repeat("a", 4096), "")
	if err != nil || len(token) > maxContinuationLength {
		t.Fatalf("4096-character cursor token length=%d err=%v", len(token), err)
	}
	if _, domain := decodeContinuation(strings.Repeat("a", maxContinuationLength+1), continuationSkills, "scope-1", ""); domain == nil || domain.Code != "INVALID_ARGUMENT" {
		t.Fatalf("oversized domain = %#v", domain)
	}
}

func FuzzDecodeContinuationNeverPanicsOrEscapesAllowlist(f *testing.F) {
	valid, _ := encodeContinuation(continuationActivity, "scope-1", "cursor-1", "session-1")
	f.Add(valid)
	f.Add("not-a-token")
	f.Fuzz(func(t *testing.T, token string) {
		cursor, domain := decodeContinuation(token, continuationActivity, "scope-1", "session-1")
		if domain == nil && cursor == "" {
			t.Fatal("successful decode returned a blank cursor")
		}
	})
}
