package main

import (
	"reflect"
	"testing"

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

func TestProjectOnlyProfileIDsSorted(t *testing.T) {
	profiles := []manifest.Profile{{ID: "omr-explore"}}
	configured := map[string]omrconfig.AgentConfig{
		"zeta":        {},
		"omr-explore": {},
		"alpha":       {},
	}
	got := projectOnlyProfileIDs(profiles, configured)
	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("project-only profiles = %#v, want %#v", got, want)
	}
}

func TestApplyProfileRoutingCopiesAndSortsState(t *testing.T) {
	item := profileJSON{ID: "omr-debug", Status: "enabled"}
	categories := map[string][]string{"omr-debug": {"z", "a"}}
	disabled := map[string]bool{"omr-debug": true}
	applyProfileRouting(&item, categories, disabled)
	if !reflect.DeepEqual(item.Categories, []string{"a", "z"}) {
		t.Fatalf("categories = %#v", item.Categories)
	}
	if !item.Disabled || item.Status != "disabled" {
		t.Fatalf("routing state = disabled:%t status:%q", item.Disabled, item.Status)
	}
	categories["omr-debug"][0] = "mutated"
	if item.Categories[0] != "a" {
		t.Fatal("categories were not copied")
	}
}
