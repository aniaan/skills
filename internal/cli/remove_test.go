package cli

import (
	"strings"
	"testing"

	"github.com/aniaan/skills/internal/skill"
	"github.com/aniaan/skills/internal/store"
)

func TestResolveTargets(t *testing.T) {
	// Anything this tool did not install can have an unsanitized directory name.
	installed := []store.Installed{
		{Skill: skill.Skill{Name: "alpha"}, Locations: []string{".agents", ".claude"}},
		{Skill: skill.Skill{Name: "MySkill"}, Locations: []string{".claude"}},
	}

	tests := []struct {
		label   string
		args    []string
		want    []string
		wantErr bool
	}{
		{label: "exact", args: []string{"alpha"}, want: []string{"alpha"}},
		{
			// Sanitizing the argument turned this into "myskill", matching
			// nothing — so `list` printed a name `remove` then rejected.
			label: "exact match on an unsanitized directory name",
			args:  []string{"MySkill"},
			want:  []string{"MySkill"},
		},
		{
			// Fail fast: the user types what list shows, and a near miss is a
			// typo rather than an invitation to pick something adjacent.
			label:   "near miss is not guessed at",
			args:    []string{"myskill"},
			wantErr: true,
		},
		{label: "duplicates collapse", args: []string{"alpha", "alpha"}, want: []string{"alpha"}},
		{label: "unknown", args: []string{"nope"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got, err := resolveTargets(installed, tt.args, store.Scope{})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveTargets(%v) succeeded, want an error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTargets: %v", err)
			}
			var names []string
			for _, sk := range got {
				names = append(names, sk.Name)
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("targets = %v, want %v", names, tt.want)
			}
		})
	}
}

// Nothing may be deleted when a later argument is bad, so the whole command is
// rejected before the first removal.
func TestResolveTargetsValidatesEverythingFirst(t *testing.T) {
	installed := []store.Installed{{Skill: skill.Skill{Name: "alpha"}}}

	if _, err := resolveTargets(installed, []string{"alpha", "nope"}, store.Scope{}); err == nil {
		t.Fatal("resolveTargets succeeded, want an error")
	}
}

// The paths offered for confirmation must be the ones that actually hold a copy.
func TestResolveTargetsCarriesLocations(t *testing.T) {
	installed := []store.Installed{
		{Skill: skill.Skill{Name: "claude-only"}, Locations: []string{".claude"}},
	}

	got, err := resolveTargets(installed, []string{"claude-only"}, store.Scope{})
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(got) != 1 || strings.Join(got[0].Locations, ",") != ".claude" {
		t.Errorf("locations = %v, want [.claude]", got[0].Locations)
	}
}
