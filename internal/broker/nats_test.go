package broker

import (
	"testing"

	"github.com/nats-io/nats.go"
)

const (
	testSubject        = "foo.bar.baz"
	testSubjectWithQux = "foo.bar.qux"
	testSubjectShort   = "foo.bar"
	testSubjectSingle  = "foo"
	testOtherSubject   = "bar"
	testWildcardMiddle = "foo.*.baz"
	testWildcardBegin  = "*.bar.baz"
	testWildcardEnd    = "foo.bar.*"
	testGtWildcard     = "foo.>"
	testGtWildcardDeep = "foo.bar.>"
)

func TestSubjectMatchesPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		subject string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: testSubject,
			subject: testSubject,
			want:    true,
		},
		{
			name:    "exact no match",
			pattern: testSubject,
			subject: testSubjectWithQux,
			want:    false,
		},
		{
			name:    "wildcard single token middle",
			pattern: testWildcardMiddle,
			subject: testSubject,
			want:    true,
		},
		{
			name:    "wildcard single token beginning",
			pattern: testWildcardBegin,
			subject: testSubject,
			want:    true,
		},
		{
			name:    "wildcard single token end",
			pattern: testWildcardEnd,
			subject: testSubject,
			want:    true,
		},
		{
			name:    "wildcard does not match multiple tokens",
			pattern: testWildcardMiddle,
			subject: "foo.bar.qux.baz",
			want:    false,
		},
		{
			name:    "gt wildcard matches rest",
			pattern: testGtWildcard,
			subject: "foo.bar.baz.qux",
			want:    true,
		},
		{
			name:    "gt wildcard matches single remaining",
			pattern: testGtWildcard,
			subject: testSubjectShort,
			want:    true,
		},
		{
			name:    "gt wildcard does not match zero tokens",
			pattern: testGtWildcardDeep,
			subject: testSubjectShort,
			want:    false,
		},
		{
			name:    "subject longer than pattern",
			pattern: testSubjectShort,
			subject: testSubject,
			want:    false,
		},
		{
			name:    "pattern longer than subject",
			pattern: testSubject,
			subject: testSubjectShort,
			want:    false,
		},
		{
			name:    "single token match",
			pattern: testSubjectSingle,
			subject: testSubjectSingle,
			want:    true,
		},
		{
			name:    "single token no match",
			pattern: testSubjectSingle,
			subject: testOtherSubject,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subjectMatchesPattern(tt.pattern, tt.subject)
			if got != tt.want {
				t.Errorf("subjectMatchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
			}
		})
	}
}

func TestNewNatsBroker(t *testing.T) {
	t.Run("uses default URL when empty", func(t *testing.T) {
		b := NewNatsBroker("", "token")
		if b.url != nats.DefaultURL {
			t.Errorf("url = %q, want %q", b.url, nats.DefaultURL)
		}
		if b.token != "token" {
			t.Errorf("token = %q, want %q", b.token, "token")
		}
	})

	t.Run("uses provided URL", func(t *testing.T) {
		want := "nats://localhost:4222"
		b := NewNatsBroker(want, "mytoken")
		if b.url != want {
			t.Errorf("url = %q, want %q", b.url, want)
		}
		if b.token != "mytoken" {
			t.Errorf("token = %q, want %q", b.token, "mytoken")
		}
	})

	t.Run("empty token is allowed", func(t *testing.T) {
		b := NewNatsBroker("nats://localhost:4222", "")
		if b.token != "" {
			t.Errorf("token = %q, want empty", b.token)
		}
	})
}

func TestErrTimeout(t *testing.T) {
	if ErrTimeout == nil {
		t.Fatal("ErrTimeout should not be nil")
	}
	if ErrTimeout.Error() != "timeout" {
		t.Errorf("ErrTimeout.Error() = %q, want %q", ErrTimeout.Error(), "timeout")
	}
}

func TestErrNoStreamForSubject(t *testing.T) {
	if ErrNoStreamForSubject == nil {
		t.Fatal("ErrNoStreamForSubject should not be nil")
	}
}
