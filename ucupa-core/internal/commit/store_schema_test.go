package commit

import (
	"reflect"
	"testing"
	storerecord "ucupa/ucupa-core/internal/store/record"
)

func TestGreenfieldStoredSpatialSchemaSurface(t *testing.T) {
	assertStructSchema(t, reflect.TypeOf(storerecord.StoredElement{}), []string{
		`Kind json:"kind"`,
		`RevisionSerial json:"revision"`,
		`Parent json:"parent,omitempty"`,
		`Pose json:"pose,omitempty"`,
		`RangeKind json:"rangeKind,omitempty"`,
		`RadiusBits json:"radiusBits,omitempty"`,
		`Departure json:"departure,omitempty"`,
		`DepartureController json:"departureController,omitempty"`,
		`RootDeparture json:"rootDeparture,omitempty"`,
	})
	assertStructSchema(t, reflect.TypeOf(storerecord.StoredPose{}), []string{
		`Translation json:"translation"`,
		`Pivot json:"pivot"`,
		`Ratios json:"ratios"`,
	})
}

func assertStructSchema(t *testing.T, value reflect.Type, want []string) {
	t.Helper()
	got := make([]string, 0, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		got = append(got, field.Name+" "+string(field.Tag))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s schema: got %v want %v", value.Name(), got, want)
	}
}
