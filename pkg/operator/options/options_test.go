package options

import (
	"context"
	"flag"
	"testing"

	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
)

func TestValidate_AllValid(t *testing.T) {
	o := Options{
		Region:                  "ap-guangzhou",
		ClusterID:               "cls-12345",
		SecretID:                "AKIDxxx",
		SecretKey:               "secret123",
		VMMemoryOverheadPercent: 0.075,
	}
	if err := o.Validate(); err != nil {
		t.Errorf("expected no error for valid options, got %v", err)
	}
}

func TestValidate_MissingRegion(t *testing.T) {
	o := Options{
		ClusterID: "cls-12345",
		SecretID:  "AKIDxxx",
		SecretKey: "secret123",
	}
	err := o.Validate()
	if err == nil {
		t.Error("expected error for missing region")
	}
}

func TestValidate_MissingClusterID(t *testing.T) {
	o := Options{
		Region:    "ap-guangzhou",
		SecretID:  "AKIDxxx",
		SecretKey: "secret123",
	}
	err := o.Validate()
	if err == nil {
		t.Error("expected error for missing cluster-id")
	}
}

func TestValidate_MissingSecretID(t *testing.T) {
	o := Options{
		Region:    "ap-guangzhou",
		ClusterID: "cls-12345",
		SecretKey: "secret123",
	}
	err := o.Validate()
	if err == nil {
		t.Error("expected error for missing secret-id")
	}
}

func TestValidate_MissingSecretKey(t *testing.T) {
	o := Options{
		Region:    "ap-guangzhou",
		ClusterID: "cls-12345",
		SecretID:  "AKIDxxx",
	}
	err := o.Validate()
	if err == nil {
		t.Error("expected error for missing secret-key")
	}
}

func TestValidate_NegativeVMMemoryOverhead(t *testing.T) {
	o := Options{
		Region:                  "ap-guangzhou",
		ClusterID:               "cls-12345",
		SecretID:                "AKIDxxx",
		SecretKey:               "secret123",
		VMMemoryOverheadPercent: -0.1,
	}
	err := o.Validate()
	if err == nil {
		t.Error("expected error for negative VMMemoryOverheadPercent")
	}
}

func TestValidate_ZeroVMMemoryOverhead(t *testing.T) {
	o := Options{
		Region:                  "ap-guangzhou",
		ClusterID:               "cls-12345",
		SecretID:                "AKIDxxx",
		SecretKey:               "secret123",
		VMMemoryOverheadPercent: 0,
	}
	if err := o.Validate(); err != nil {
		t.Errorf("expected no error for zero VMMemoryOverheadPercent, got %v", err)
	}
}

func TestToContext_FromContext(t *testing.T) {
	opts := &Options{
		Region:    "ap-shanghai",
		ClusterID: "cls-abc",
		SecretID:  "id",
		SecretKey: "key",
	}
	ctx := ToContext(context.Background(), opts)
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil options from context")
	}
	if got.Region != "ap-shanghai" {
		t.Errorf("expected region ap-shanghai, got %s", got.Region)
	}
	if got.ClusterID != "cls-abc" {
		t.Errorf("expected cluster-id cls-abc, got %s", got.ClusterID)
	}
}

func TestFromContext_Nil(t *testing.T) {
	ctx := context.Background()
	got := FromContext(ctx)
	if got != nil {
		t.Errorf("expected nil options from empty context, got %+v", got)
	}
}

func TestOptions_ToContext(t *testing.T) {
	opts := &Options{
		Region:    "ap-beijing",
		ClusterID: "cls-xyz",
		SecretID:  "id",
		SecretKey: "key",
	}
	ctx := opts.ToContext(context.Background())
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil options from context")
	}
	if got.Region != "ap-beijing" {
		t.Errorf("expected region ap-beijing, got %s", got.Region)
	}
}

func TestAddFlags(t *testing.T) {
	o := &Options{}
	fs := &coreoptions.FlagSet{FlagSet: flag.NewFlagSet("test", flag.ContinueOnError)}
	o.AddFlags(fs)

	// Verify flags are registered
	for _, name := range []string{"region", "cluster-id", "secret-id", "secret-key", "vm-memory-overhead-percent"} {
		if fs.Lookup(name) == nil {
			t.Errorf("expected flag %q to be registered", name)
		}
	}
}

func TestParse_Success(t *testing.T) {
	o := &Options{}
	fs := &coreoptions.FlagSet{FlagSet: flag.NewFlagSet("test", flag.ContinueOnError)}
	o.AddFlags(fs)

	err := o.Parse(fs,
		"--region", "ap-guangzhou",
		"--cluster-id", "cls-test",
		"--secret-id", "AKID",
		"--secret-key", "secret",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if o.Region != "ap-guangzhou" {
		t.Errorf("expected region ap-guangzhou, got %s", o.Region)
	}
	if o.ClusterID != "cls-test" {
		t.Errorf("expected cluster-id cls-test, got %s", o.ClusterID)
	}
}

func TestParse_InvalidFlag(t *testing.T) {
	o := &Options{}
	fs := &coreoptions.FlagSet{FlagSet: flag.NewFlagSet("test", flag.ContinueOnError)}
	o.AddFlags(fs)

	err := o.Parse(fs, "--nonexistent-flag", "value")
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
}

func TestParse_ValidationFailure(t *testing.T) {
	o := &Options{}
	fs := &coreoptions.FlagSet{FlagSet: flag.NewFlagSet("test", flag.ContinueOnError)}
	o.AddFlags(fs)

	// Parse with missing required fields
	err := o.Parse(fs)
	if err == nil {
		t.Fatal("expected validation error for missing required fields")
	}
}

func TestParse_WithVMMemoryOverhead(t *testing.T) {
	o := &Options{}
	fs := &coreoptions.FlagSet{FlagSet: flag.NewFlagSet("test", flag.ContinueOnError)}
	o.AddFlags(fs)

	err := o.Parse(fs,
		"--region", "ap-shanghai",
		"--cluster-id", "cls-test",
		"--secret-id", "AKID",
		"--secret-key", "secret",
		"--vm-memory-overhead-percent", "0.1",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if o.VMMemoryOverheadPercent != 0.1 {
		t.Errorf("expected VMMemoryOverheadPercent 0.1, got %f", o.VMMemoryOverheadPercent)
	}
}
