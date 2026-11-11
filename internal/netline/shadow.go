package netline

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Difference is a forward the lines plan differently from today's routes.
type Difference struct {
	ID string `json:"id"`
	// What is "missing" (the routes plan it, the lines do not), "extra"
	// (the reverse) or the fields that differ.
	What string `json:"what"`
}

// Report is a snapshot and how its plan compares with today's.
type Report struct {
	Snapshot *Snapshot `json:"snapshot"`
	// Forwards counts the forwards the routes plan today.
	Forwards    int          `json:"forwards"`
	Differences []Difference `json:"differences"`
}

// Shadow imports the stored network and compares the forwards its lines
// plan with the ones the forward routes plan today. No difference means
// the lines would give every device the plan it has.
func Shadow(db *gorm.DB) (*Report, error) {
	snapshot, err := Import(db)
	if err != nil {
		return nil, err
	}
	today, err := networkmodel.SourceForwards("forward", db)
	if err != nil {
		return nil, err
	}
	return &Report{Snapshot: snapshot, Forwards: len(today), Differences: Compare(today, Forwards(snapshot))}, nil
}

// Compare lists how the forwards of lines differ from today's, by id.
func Compare(today, lines []networkmodel.Forward) []Difference {
	differences := []Difference{}
	planned := map[string]networkmodel.Forward{}
	for _, forward := range lines {
		planned[forward.ID] = forward
	}
	for _, forward := range today {
		other, ok := planned[forward.ID]
		delete(planned, forward.ID)
		if !ok {
			differences = append(differences, Difference{ID: forward.ID, What: "missing"})
		} else if fields := differing(forward, other); len(fields) > 0 {
			differences = append(differences, Difference{ID: forward.ID, What: fmt.Sprint(fields)})
		}
	}
	for id := range planned {
		differences = append(differences, Difference{ID: id, What: "extra"})
	}
	sort.Slice(differences, func(i, j int) bool { return differences[i].ID < differences[j].ID })
	return differences
}

// differing names the fields of two forwards that differ. Empty and nil
// lists are the same, and messages compare as messages.
func differing(a, b networkmodel.Forward) []string {
	var fields []string
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < va.NumField(); i++ {
		if !same(va.Field(i), vb.Field(i)) {
			fields = append(fields, va.Type().Field(i).Name)
		}
	}
	return fields
}

func same(a, b reflect.Value) bool {
	if !a.CanInterface() {
		return true
	}
	if message, ok := a.Interface().(proto.Message); ok {
		return proto.Equal(message, b.Interface().(proto.Message))
	}
	switch a.Kind() {
	case reflect.Slice:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !same(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !same(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return same(a.Elem(), b.Elem())
	}
	return a.Interface() == b.Interface()
}
