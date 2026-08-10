package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"traversal", "../../../etc/passwd", "etc-passwd"},
		{"absolute", "/etc/shadow", "etc-shadow"},
		{"deep traversal", "../../../../../../tmp/PWNED", "tmp-pwned"},
		{"leading dot", ".hidden", "hidden"},
		{"trailing hyphens", "name---", "name"},
		{"only slashes", "///", "unnamed-skill"},
		{"empty", "", "unnamed-skill"},
		{"nul byte", "a\x00b", "a-b"},
		{"non-ascii", "设计规范", "unnamed-skill"},
		{"spaces and case", "Convex Best Practices", "convex-best-practices"},
		{"underscore kept", "my_skill", "my_skill"},
		{"dots kept", "v1.2.skill", "v1.2.skill"},
		{"case only", "WebDesign", "webdesign"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeName(tt.in); got != tt.want {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeNameTruncates(t *testing.T) {
	got := SanitizeName(strings.Repeat("a", 300))
	if len(got) != 255 {
		t.Errorf("len = %d, want 255", len(got))
	}
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		label   string
		raw     string
		name    string
		desc    string
		wantErr bool
	}{
		{
			label: "plain",
			raw:   "---\nname: foo\ndescription: does a thing\n---\n# Foo\n",
			name:  "foo",
			desc:  "does a thing",
		},
		{
			label: "quoted with colon",
			raw:   "---\nname: foo\ndescription: \"use this: always\"\n---\n",
			name:  "foo",
			desc:  "use this: always",
		},
		{
			label: "single quoted escape",
			raw:   "---\nname: foo\ndescription: 'it''s fine'\n---\n",
			name:  "foo",
			desc:  "it's fine",
		},
		{
			label: "folded scalar",
			raw:   "---\nname: foo\ndescription: >-\n  line one\n  line two\n---\n",
			name:  "foo",
			desc:  "line one line two",
		},
		{
			label: "literal scalar",
			raw:   "---\nname: foo\ndescription: |\n  line one\n  line two\n---\n",
			name:  "foo",
			desc:  "line one\nline two",
		},
		{
			label: "extra fields ignored",
			raw:   "---\nname: foo\nallowed-tools: [Bash, Read]\nuser-invocable: true\ndescription: d\n---\n",
			name:  "foo",
			desc:  "d",
		},
		{
			label: "crlf",
			raw:   "---\r\nname: foo\r\ndescription: d\r\n---\r\n",
			name:  "foo",
			desc:  "d",
		},
		{
			label: "bom",
			raw:   "\ufeff---\nname: foo\ndescription: d\n---\n",
			name:  "foo",
			desc:  "d",
		},
		{
			label: "no frontmatter",
			raw:   "# Just a heading\n",
		},
		{
			label: "unterminated fence",
			raw:   "---\nname: foo\n",
		},
		{
			label:   "malformed yaml",
			raw:     "---\nname: [unclosed\n---\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			name, desc, err := ParseFrontmatter([]byte(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tt.name {
				t.Errorf("name = %q, want %q", name, tt.name)
			}
			if desc != tt.desc {
				t.Errorf("description = %q, want %q", desc, tt.desc)
			}
		})
	}
}

func TestLoadFallsBackToDirName(t *testing.T) {
	dir := t.TempDir()
	sub := mkSkill(t, dir, "My Skill", "# no frontmatter\n")

	sk, err := Load(sub)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sk.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", sk.Name, "my-skill")
	}
}

func TestLoadPrefersFrontmatterName(t *testing.T) {
	dir := t.TempDir()
	sub := mkSkill(t, dir, "dir-name", "---\nname: Frontmatter Name\ndescription: d\n---\n")

	sk, err := Load(sub)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sk.Name != "frontmatter-name" {
		t.Errorf("Name = %q, want %q", sk.Name, "frontmatter-name")
	}
	if sk.Description != "d" {
		t.Errorf("Description = %q, want %q", sk.Description, "d")
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	mkSkill(t, root, "beta", "---\nname: beta\n---\n")
	mkSkill(t, root, "alpha", "---\nname: alpha\n---\n")
	mkSkill(t, filepath.Join(root, ".git"), "ignored", "---\nname: ignored\n---\n")

	// A SKILL.md inside a skill is a supporting file, not a second skill.
	nested := filepath.Join(root, "alpha", "reference")
	mkSkill(t, filepath.Dir(nested), filepath.Base(nested), "---\nname: nested\n---\n")

	skills, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var names []string
	for _, sk := range skills {
		names = append(names, sk.Name)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("names = %v, want [alpha beta]", names)
	}
}

func TestDiscoverRootIsSkill(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, File), []byte("---\nname: solo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .git at the repository root must not stop the root itself being a skill.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	skills, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "solo" {
		t.Fatalf("got %+v, want one skill named solo", skills)
	}
}

func mkSkill(t *testing.T, parent, name, content string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, File), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
