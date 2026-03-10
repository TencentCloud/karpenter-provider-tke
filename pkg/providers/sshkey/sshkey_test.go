package sshkey

import (
	"testing"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
)

func TestGetFilterSets_IDFilter(t *testing.T) {
	terms := []api.SSHKeySelectorTerm{
		{ID: "skey-12345"},
		{ID: "skey-67890"},
	}
	ids, filterSets := getFilterSets(terms)
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
	if lo.FromPtr(ids[0]) != "skey-12345" {
		t.Errorf("expected skey-12345, got %s", lo.FromPtr(ids[0]))
	}
	if lo.FromPtr(ids[1]) != "skey-67890" {
		t.Errorf("expected skey-67890, got %s", lo.FromPtr(ids[1]))
	}
	if len(filterSets) != 0 {
		t.Errorf("expected 0 filter sets for ID-only terms, got %d", len(filterSets))
	}
}

func TestGetFilterSets_TagFilter(t *testing.T) {
	terms := []api.SSHKeySelectorTerm{
		{Tags: map[string]string{"env": "prod"}},
	}
	ids, filterSets := getFilterSets(terms)
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	if len(filterSets[0]) != 1 {
		t.Fatalf("expected 1 filter in set, got %d", len(filterSets[0]))
	}
	filter := filterSets[0][0]
	if lo.FromPtr(filter.Name) != "tag:env" {
		t.Errorf("expected filter name 'tag:env', got %q", lo.FromPtr(filter.Name))
	}
	if lo.FromPtr(filter.Values[0]) != "prod" {
		t.Errorf("expected filter value 'prod', got %q", lo.FromPtr(filter.Values[0]))
	}
}

func TestGetFilterSets_TagWildcard(t *testing.T) {
	terms := []api.SSHKeySelectorTerm{
		{Tags: map[string]string{"karpenter": "*"}},
	}
	ids, filterSets := getFilterSets(terms)
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	filter := filterSets[0][0]
	if lo.FromPtr(filter.Name) != "tag-key" {
		t.Errorf("expected filter name 'tag-key', got %q", lo.FromPtr(filter.Name))
	}
	if lo.FromPtr(filter.Values[0]) != "karpenter" {
		t.Errorf("expected filter value 'karpenter', got %q", lo.FromPtr(filter.Values[0]))
	}
}

func TestGetFilterSets_Mixed(t *testing.T) {
	terms := []api.SSHKeySelectorTerm{
		{ID: "skey-12345"},
		{Tags: map[string]string{"env": "prod"}},
		{Tags: map[string]string{"team": "*"}},
	}
	ids, filterSets := getFilterSets(terms)
	if len(ids) != 1 {
		t.Errorf("expected 1 ID, got %d", len(ids))
	}
	if len(filterSets) != 2 {
		t.Errorf("expected 2 filter sets, got %d", len(filterSets))
	}
}

func TestGetFilterSets_Empty(t *testing.T) {
	ids, filterSets := getFilterSets(nil)
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
	if len(filterSets) != 0 {
		t.Errorf("expected 0 filter sets, got %d", len(filterSets))
	}
}

func TestGetFilterSets_MultipleTags(t *testing.T) {
	terms := []api.SSHKeySelectorTerm{
		{Tags: map[string]string{"env": "prod", "team": "infra"}},
	}
	ids, filterSets := getFilterSets(terms)
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	if len(filterSets[0]) != 2 {
		t.Errorf("expected 2 filters for 2 tags, got %d", len(filterSets[0]))
	}
}
