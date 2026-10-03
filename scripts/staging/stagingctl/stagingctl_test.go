package main

import (
	"strings"
	"testing"
	"time"
)

func TestEveryPackageWithNativeRoutesIsInOneBatch(t *testing.T) {
	if err := checkBatchCoverage(); err != nil {
		t.Fatal(err)
	}
}

// TestEveryBatchRouteHasAGenerator keeps the rehearsal complete: a new
// native-flagged route needs a request generator in its batch file.
func TestEveryBatchRouteHasAGenerator(t *testing.T) {
	world := newWorld(1)
	for key := range map[string]bool{Admin: true, Admin2: true, Staff: true, User: true, User2: true, Fresh: true, Banned: true, Expired: true} {
		world.Personas[key] = Persona{Email: key + "@example.com", ID: 1}
	}
	known := map[string]bool{Anon: true}
	for key := range world.Personas {
		known[key] = true
	}
	for _, batch := range Batches {
		routes, err := batchRoutes(batch)
		if err != nil {
			t.Fatal(err)
		}
		if missing := specCoverage(routes); len(missing) > 0 {
			t.Errorf("batch %d: routes without a generator: %s", batch.Number, strings.Join(missing, ", "))
		}
		for _, route := range routes {
			spec, ok := routeSpecs[route.RouteID]
			if !ok {
				continue
			}
			var requests []Req
			switch {
			case spec.Skip != "":
				continue
			case route.Read() && spec.Reads == nil:
				t.Errorf("%s is a GET route without Reads", route.RouteID)
			case !route.Read() && spec.Writes == nil:
				t.Errorf("%s is a %s route without Writes", route.RouteID, route.Method)
			case route.Read():
				requests = spec.Reads(world)
			default:
				requests = spec.Writes(world)
			}
			if len(requests) == 0 {
				t.Errorf("%s generates no request", route.RouteID)
			}
			for _, request := range requests {
				if !known[request.Persona] {
					t.Errorf("%s: unknown persona %q", route.RouteID, request.Persona)
				}
				if !strings.HasPrefix(request.Path, "/api/v2/") || strings.Contains(request.Path, "/:") {
					t.Errorf("%s: path %q is not a filled /api/v2 path", route.RouteID, request.Path)
				}
				_ = request.BodyBytes()
			}
		}
	}
}

func TestMaskJSON(t *testing.T) {
	body := []byte(`{"code":0,"data":{"items":[{"id":1,"created_at":"a"},{"id":2,"created_at":"b"}],"token":"x"}}`)
	got := string(maskJSON(body, []string{"data.items.*.created_at", "data.token"}))
	want := `{"code":0,"data":{"items":[{"created_at":"(masked)","id":1},{"created_at":"(masked)","id":2}],"token":"(masked)"}}`
	if got != want {
		t.Fatalf("maskJSON = %s, want %s", got, want)
	}
	if string(maskJSON([]byte("not json"), []string{"a"})) != "not json" {
		t.Fatal("non-JSON bodies must pass through")
	}
}

func TestShadowSinceUsesTheLatestRevisionOfEachRoute(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	since := shadowSince([]routeModeRevision{ // newest first
		{RouteID: "a", ToMode: "shadow", CreatedAt: now},
		{RouteID: "b", ToMode: "legacy", CreatedAt: now},
		{RouteID: "a", ToMode: "legacy", CreatedAt: now.Add(-time.Hour)},
		{RouteID: "b", ToMode: "shadow", CreatedAt: now.Add(-2 * time.Hour)},
		{RouteID: "c", ToMode: "shadow", CreatedAt: now.Add(-3 * time.Hour)},
	})
	if !since["a"].Equal(now) || !since["c"].Equal(now.Add(-3*time.Hour)) {
		t.Fatalf("since = %v", since)
	}
	if _, ok := since["b"]; ok {
		t.Fatal("a route switched away from shadow has no shadow start")
	}
}

func TestDiffRowsCountsDifferencesAndChanges(t *testing.T) {
	result := TableResult{Key: "id"}
	diffRows(&result,
		map[string]string{"1": `{"a":1}`, "2": `{"a":2}`, "3": `{"a":3}`},
		map[string]string{"1": `{"a":1}`, "2": `{"a":9}`, "4": `{"a":4}`},
		map[string]string{"1": `{"a":1}`, "2": `{"a":2}`})
	if result.Differences != 3 || result.ChangedRows != 3 || len(result.Samples) != 3 {
		t.Fatalf("result = %+v", result)
	}
}
