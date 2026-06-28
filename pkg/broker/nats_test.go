package broker

import "testing"

func TestSubjectMatchesPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		subject string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "foo.bar.baz",
			subject: "foo.bar.baz",
			want:    true,
		},
		{
			name:    "exact no match",
			pattern: "foo.bar.baz",
			subject: "foo.bar.qux",
			want:    false,
		},
		{
			name:    "wildcard single token middle",
			pattern: "foo.*.baz",
			subject: "foo.bar.baz",
			want:    true,
		},
		{
			name:    "wildcard single token beginning",
			pattern: "*.bar.baz",
			subject: "foo.bar.baz",
			want:    true,
		},
		{
			name:    "wildcard single token end",
			pattern: "foo.bar.*",
			subject: "foo.bar.baz",
			want:    true,
		},
		{
			name:    "wildcard does not match multiple tokens",
			pattern: "foo.*.baz",
			subject: "foo.bar.qux.baz",
			want:    false,
		},
		{
			name:    "gt wildcard matches rest",
			pattern: "foo.>",
			subject: "foo.bar.baz.qux",
			want:    true,
		},
		{
			name:    "gt wildcard matches single remaining",
			pattern: "foo.>",
			subject: "foo.bar",
			want:    true,
		},
		{
			name:    "gt wildcard does not match zero tokens",
			pattern: "foo.bar.>",
			subject: "foo.bar",
			want:    false,
		},
		{
			name:    "subject longer than pattern",
			pattern: "foo.bar",
			subject: "foo.bar.baz",
			want:    false,
		},
		{
			name:    "pattern longer than subject",
			pattern: "foo.bar.baz",
			subject: "foo.bar",
			want:    false,
		},
		{
			name:    "single token match",
			pattern: "foo",
			subject: "foo",
			want:    true,
		},
		{
			name:    "single token no match",
			pattern: "foo",
			subject: "bar",
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
