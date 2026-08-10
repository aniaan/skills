package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/aniaan/skills/internal/skill"
)

func TestSelectSkills(t *testing.T) {
	alpha := skill.Skill{Name: "alpha", Dir: "/src/alpha"}
	beta := skill.Skill{Name: "beta", Dir: "/src/beta"}
	// Frontmatter and directory name disagree, so both must resolve.
	gamma := skill.Skill{Name: "gamma", Dir: "/src/gamma-dir"}

	tests := []struct {
		label   string
		found   []skill.Skill
		names   []string
		all     bool
		want    []string
		wantErr string
	}{
		{
			label: "all",
			found: []skill.Skill{alpha, beta},
			all:   true,
			want:  []string{"alpha", "beta"},
		},
		{
			label: "lone skill needs no name",
			found: []skill.Skill{alpha},
			want:  []string{"alpha"},
		},
		{
			label:   "ambiguous lists candidates",
			found:   []skill.Skill{alpha, beta},
			wantErr: "contains 2 skills",
		},
		{
			label: "by frontmatter name",
			found: []skill.Skill{alpha, beta},
			names: []string{"beta"},
			want:  []string{"beta"},
		},
		{
			label: "by directory name",
			found: []skill.Skill{alpha, gamma},
			names: []string{"gamma-dir"},
			want:  []string{"gamma"},
		},
		{
			label: "name is sanitized before matching",
			found: []skill.Skill{alpha, beta},
			names: []string{"Beta"},
			want:  []string{"beta"},
		},
		{
			label: "duplicates collapse",
			found: []skill.Skill{alpha, beta},
			names: []string{"alpha", "alpha"},
			want:  []string{"alpha"},
		},
		{
			label:   "unknown name",
			found:   []skill.Skill{alpha, beta},
			names:   []string{"nope"},
			wantErr: `"nope" not found`,
		},
		{
			label:   "nothing found",
			found:   nil,
			wantErr: "no SKILL.md found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got, err := selectSkills(tt.found, tt.names, tt.all, "owner/repo")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("selectSkills succeeded, want an error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectSkills: %v", err)
			}

			var names []string
			for _, sk := range got {
				names = append(names, sk.Name)
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("selected %v, want %v", names, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
	// Newlines in a description would break the one-line list layout.
	if got := truncate("a\n  b\tc", 10); got != "a b c" {
		t.Errorf("got %q, want %q", got, "a b c")
	}
	if got := truncate("abcdefghij", 5); got != "abcd…" {
		t.Errorf("got %q, want %q", got, "abcd…")
	}
}

// Budgeting in bytes cut a description mid-rune, printing a replacement
// character, and spent a CJK description's budget three times over.
func TestTruncateCountsRunes(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"🚀🚀🚀🚀", 7, "🚀🚀🚀🚀"},
		{"🚀🚀🚀🚀", 3, "🚀🚀…"},
		{"中文描述中文描述", 8, "中文描述中文描述"},
		{"中文描述中文描述", 4, "中文描…"},
		{"中文", 0, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.in, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("truncate(%q, %d) = %q, which is not valid UTF-8", tt.in, tt.max, got)
		}
	}
}
