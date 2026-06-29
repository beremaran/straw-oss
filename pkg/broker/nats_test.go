package broker

import "testing"

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
