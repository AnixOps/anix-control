package validate

import (
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"google.golang.org/protobuf/proto"
)

// FuzzValidateRoute feeds arbitrary encoded routes through validation. It
// must never panic, and every violation must carry a code and a message.
// Run it longer with: go test ./forward/validate -fuzz FuzzValidateRoute
func FuzzValidateRoute(f *testing.F) {
	seeds := []model.Route{validRoute()}
	for _, tc := range ruleCases() {
		r := validRoute()
		var opts Options
		tc.edit(&r, &opts)
		seeds = append(seeds, r)
	}
	for i := range seeds {
		// Routes proto cannot encode (invalid UTF-8) are no seeds.
		if raw, err := proto.Marshal(seeds[i].ToProto()); err == nil {
			f.Add(raw)
		}
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		route := &forwardv1.Route{}
		if err := proto.Unmarshal(raw, route); err != nil {
			return
		}
		for _, opts := range []Options{
			{},
			{Nodes: testNodes(), EnableAnixOps: true, OnCreate: true, Now: time.UnixMilli(1790000000000),
				ReservedPorts: map[string][]uint32{"forward-21": {22}}},
		} {
			for _, v := range Proto(route, opts) {
				if v.Code == "" || v.Message == "" {
					t.Fatalf("violation without code or message: %+v", v)
				}
			}
		}
	})
}
