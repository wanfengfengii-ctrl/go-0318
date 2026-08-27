package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"abyssal-pressure-housing-qualification/configuration"
)

func TestModel_FrozenConfigurationOwnsNestedCollections(t *testing.T) {
	cloneInput := func(t *testing.T, in configuration.Input) configuration.Input {
		t.Helper()
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal input: %v", err)
		}
		var clone configuration.Input
		if err := json.Unmarshal(raw, &clone); err != nil {
			t.Fatalf("unmarshal input: %v", err)
		}
		return clone
	}

	cases := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "Freeze neither rewrites nor aliases caller collections",
			run: func(t *testing.T) {
				in := testConfig()
				in.SealBoundaries[0].Checks = []string{"密封复查", "外观检查"}
				before := cloneInput(t, in)

				snap, err := configuration.Freeze(in)
				if err != nil {
					t.Fatalf("Freeze: %v", err)
				}
				if !reflect.DeepEqual(in, before) {
					t.Fatalf("Freeze modified its input\ngot:  %#v\nwant: %#v", in, before)
				}

				in.Chambers[0].Name = "reused chamber"
				in.Ports[0].ID = "reused port"
				in.Pipes[0].ID = "reused pipe"
				in.SealBoundaries[0].ID = "reused seal"
				in.SealBoundaries[0].Checks[0] = "reused check"
				in.Steps[0].TargetPa = 1
				in.Calibrations[0].Serial = "reused calibration"

				want, err := configuration.Freeze(before)
				if err != nil {
					t.Fatalf("Freeze pristine input: %v", err)
				}
				if !reflect.DeepEqual(snap, want) {
					t.Fatalf("snapshot changed after caller reused input\ngot:  %#v\nwant: %#v", snap, want)
				}
			},
		},
		{
			name: "service result remains equal to its persisted digest lookup",
			run: func(t *testing.T) {
				svc := newTestService(t)
				in := testConfig()
				in.SealBoundaries[0].Checks = []string{"密封复查", "外观检查"}
				before := cloneInput(t, in)

				returned, err := svc.FreezeConfiguration(context.Background(), in)
				if err != nil {
					t.Fatalf("FreezeConfiguration: %v", err)
				}
				if !reflect.DeepEqual(in, before) {
					t.Fatalf("FreezeConfiguration modified its input\ngot:  %#v\nwant: %#v", in, before)
				}

				in.Chambers[0].Name = "caller reused chamber"
				in.Ports[0].Channel = "caller reused channel"
				in.Pipes[0].From = "caller reused endpoint"
				in.SealBoundaries[0].Checks[0] = "caller reused check"
				in.Steps[0].HoldMs = 1
				in.Calibrations[0].Summary = "caller reused calibration"

				persisted, err := svc.GetConfiguration(context.Background(), returned.Digest)
				if err != nil {
					t.Fatalf("GetConfiguration(%q): %v", returned.Digest, err)
				}
				if !reflect.DeepEqual(returned, persisted) {
					t.Fatalf("returned snapshot diverged from digest lookup\nreturned:  %#v\npersisted: %#v", returned, persisted)
				}
			},
		},
		{
			name: "inspection checks are canonical but still affect the digest",
			run: func(t *testing.T) {
				first := testConfig()
				first.SealBoundaries[0].Checks = []string{"密封复查", "外观检查"}
				second := cloneInput(t, first)
				second.SealBoundaries[0].Checks[0], second.SealBoundaries[0].Checks[1] = second.SealBoundaries[0].Checks[1], second.SealBoundaries[0].Checks[0]

				one, err := configuration.Freeze(first)
				if err != nil {
					t.Fatalf("Freeze first ordering: %v", err)
				}
				two, err := configuration.Freeze(second)
				if err != nil {
					t.Fatalf("Freeze second ordering: %v", err)
				}
				if one.Digest != two.Digest {
					t.Fatalf("equivalent check orderings produced different digests: %q != %q", one.Digest, two.Digest)
				}

				changed := cloneInput(t, first)
				changed.SealBoundaries[0].Checks[0] = "扭矩复核"
				three, err := configuration.Freeze(changed)
				if err != nil {
					t.Fatalf("Freeze changed checks: %v", err)
				}
				if one.Digest == three.Digest {
					t.Fatalf("different inspection checks produced the same digest %q", one.Digest)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}
