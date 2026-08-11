package source

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		label   string
		in      string
		clone   string
		ref     string // parsed, then deliberately ignored by Fetch
		sub     string
		filter  string
		local   string
		wantErr bool
	}{
		{
			label: "shorthand",
			in:    "vercel-labs/agent-skills",
			clone: "https://github.com/vercel-labs/agent-skills.git",
		},
		{
			label:  "shorthand with skill",
			in:     "vercel-labs/agent-skills@web-design-guidelines",
			clone:  "https://github.com/vercel-labs/agent-skills.git",
			filter: "web-design-guidelines",
		},
		{
			label: "https url",
			in:    "https://github.com/owner/repo",
			clone: "https://github.com/owner/repo.git",
		},
		{
			label: "https url with .git",
			in:    "https://github.com/owner/repo.git",
			clone: "https://github.com/owner/repo.git",
		},
		{
			label: "https url with trailing slash",
			in:    "https://github.com/owner/repo/",
			clone: "https://github.com/owner/repo.git",
		},
		{
			label: "tree url with ref and subpath",
			in:    "https://github.com/owner/repo/tree/main/skills/foo",
			clone: "https://github.com/owner/repo.git",
			ref:   "main",
			sub:   "skills/foo",
		},
		{
			label: "tree url with ref only",
			in:    "https://github.com/owner/repo/tree/v1.2.3",
			clone: "https://github.com/owner/repo.git",
			ref:   "v1.2.3",
		},
		{
			// A permalink pins a SHA: parsed like any ref, discarded like any
			// ref. It used to become `git clone --branch <sha>`, which fails.
			label: "tree url with commit sha",
			in:    "https://github.com/owner/repo/tree/c0f91d41c28b02c8142a3161fbbb089bab35f051/skills/foo",
			clone: "https://github.com/owner/repo.git",
			ref:   "c0f91d41c28b02c8142a3161fbbb089bab35f051",
			sub:   "skills/foo",
		},
		{
			// An escaping subpath degrades to "search the whole repo" rather
			// than reaching outside the clone.
			label: "tree url with escaping subpath",
			in:    "https://github.com/owner/repo/tree/main/../../etc",
			clone: "https://github.com/owner/repo.git",
			ref:   "main",
			sub:   "",
		},
		{
			// git+ is the default transport written out, and changes nothing.
			label: "explicit git prefix",
			in:    "git+https://github.com/owner/repo",
			clone: "https://github.com/owner/repo.git",
		},
		{
			label: "explicit git prefix on shorthand",
			in:    "git+owner/repo",
			clone: "https://github.com/owner/repo.git",
		},
		{
			// The @ in git@host must not be read as a skill filter.
			label: "ssh url",
			in:    "git@github.com:owner/repo.git",
			clone: "git@github.com:owner/repo.git",
		},
		{
			label: "gitlab passthrough",
			in:    "https://gitlab.com/group/sub/repo.git",
			clone: "https://gitlab.com/group/sub/repo.git",
		},
		{
			label: "self-hosted ssh scheme",
			in:    "ssh://git@git.example.com:2222/team/repo.git",
			clone: "ssh://git@git.example.com:2222/team/repo.git",
		},
		{label: "relative path", in: "./skills", local: "./skills"},
		{label: "parent path", in: "../skills", local: "../skills"},
		{label: "dot", in: ".", local: "."},
		{label: "absolute path", in: "/srv/skills", local: "/srv/skills"},
		{label: "home path", in: "~/skills", local: "~/skills"},
		// A local path may legitimately contain @, so no filter is split off.
		{label: "local path with at", in: "./my@dir", local: "./my@dir"},
		{label: "empty", in: "", wantErr: true},
		{label: "gibberish", in: "not a source", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want an error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got.CloneURL != tt.clone {
				t.Errorf("CloneURL = %q, want %q", got.CloneURL, tt.clone)
			}
			if got.IgnoredRef != tt.ref {
				t.Errorf("IgnoredRef = %q, want %q", got.IgnoredRef, tt.ref)
			}
			if got.Subpath != tt.sub {
				t.Errorf("Subpath = %q, want %q", got.Subpath, tt.sub)
			}
			if got.SkillFilter != tt.filter {
				t.Errorf("SkillFilter = %q, want %q", got.SkillFilter, tt.filter)
			}
			if got.Local != tt.local {
				t.Errorf("Local = %q, want %q", got.Local, tt.local)
			}
			if got.IsLocal() != (tt.local != "") {
				t.Errorf("IsLocal() = %v, want %v", got.IsLocal(), tt.local != "")
			}
			if got.Display != tt.in {
				t.Errorf("Display = %q, want %q", got.Display, tt.in)
			}
		})
	}
}
