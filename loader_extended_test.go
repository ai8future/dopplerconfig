package dopplerconfig

import (
	"context"
	"testing"
)

type SliceConfig struct {
	Ints  []int  `doppler:"INTS"`
	Bools []bool `doppler:"BOOLS"`
}

func TestLoader_ExtendedSlices(t *testing.T) {
	values := map[string]string{
		"INTS":  "1, 2, 3, 4",
		"BOOLS": "true, false, 1, 0, yes, no",
	}

	loader, _ := TestLoader[SliceConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	expectedInts := []int{1, 2, 3, 4}
	if len(cfg.Ints) != len(expectedInts) {
		t.Errorf("Ints length = %d, want %d", len(cfg.Ints), len(expectedInts))
	}
	for i, v := range expectedInts {
		if cfg.Ints[i] != v {
			t.Errorf("Ints[%d] = %d, want %d", i, cfg.Ints[i], v)
		}
	}

	expectedBools := []bool{true, false, true, false, true, false}
	if len(cfg.Bools) != len(expectedBools) {
		t.Errorf("Bools length = %d, want %d", len(cfg.Bools), len(expectedBools))
	}
	for i, v := range expectedBools {
		if cfg.Bools[i] != v {
			t.Errorf("Bools[%d] = %v, want %v", i, cfg.Bools[i], v)
		}
	}
}
