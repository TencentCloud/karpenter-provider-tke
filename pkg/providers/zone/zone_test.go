package zone

import (
	"context"
	"testing"
)

func TestZoneFromID_Valid(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	// ap-guangzhou zone base is 100000, so 100001 => ap-guangzhou-1
	zone, err := p.ZoneFromID("100001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if zone != "ap-guangzhou-1" {
		t.Errorf("expected ap-guangzhou-1, got %s", zone)
	}
}

func TestZoneFromID_ValidOtherRegion(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	// ap-shanghai zone base is 200000, so 200003 => ap-shanghai-3
	zone, err := p.ZoneFromID("200003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if zone != "ap-shanghai-3" {
		t.Errorf("expected ap-shanghai-3, got %s", zone)
	}
}

func TestZoneFromID_NonNumeric(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	_, err := p.ZoneFromID("abc")
	if err == nil {
		t.Error("expected error for non-numeric ID")
	}
}

func TestZoneFromID_NotInRange(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	_, err := p.ZoneFromID("999999")
	if err == nil {
		t.Error("expected error for ID not matching any zone group")
	}
}

func TestIDFromZone_ThreeSegments(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	// ap-guangzhou-3 => 100000 + 3 = 100003
	id, err := p.IDFromZone("ap-guangzhou-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "100003" {
		t.Errorf("expected 100003, got %s", id)
	}
}

func TestIDFromZone_FourSegments(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	// ap-shenzhen-fsi-1 => 4 segments: region=ap-shenzhen-fsi, zone suffix=1
	// ap-shenzhen-fsi base is 110000, so 110000 + 1 = 110001
	id, err := p.IDFromZone("ap-shenzhen-fsi-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "110001" {
		t.Errorf("expected 110001, got %s", id)
	}
}

func TestIDFromZone_TooFewSegments(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	_, err := p.IDFromZone("ap-guangzhou")
	if err == nil {
		t.Error("expected error for zone with fewer than 3 segments")
	}
}

func TestIDFromZone_NonExistentRegion(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	_, err := p.IDFromZone("ap-nonexistent-1")
	if err == nil {
		t.Error("expected error for non-existent region")
	}
}

func TestIDFromZone_NonNumericSuffix(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	_, err := p.IDFromZone("ap-guangzhou-abc")
	if err == nil {
		t.Error("expected error for non-numeric zone suffix")
	}
}

func TestIDFromZone_SingleSegment(t *testing.T) {
	p := NewDefaultProvider(context.Background())
	_, err := p.IDFromZone("guangzhou")
	if err == nil {
		t.Error("expected error for single segment zone")
	}
}
