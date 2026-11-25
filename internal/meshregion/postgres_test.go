package meshregion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The address choices of the mesh rely on row and advisory locks that
// only PostgreSQL takes (SQLite writes one transaction at a time). These
// tests run the changes at the same time on PostgreSQL; CI sets
// ANIX_TEST_POSTGRES_DSN (the PostgreSQL package jobs).

// openPostgres opens the mesh's tables in a new schema of the test
// database, dropped when the test ends.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })
	quiet := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin := func(statement string) error {
		db, err := gorm.Open(postgres.Open(base), quiet)
		if err != nil {
			return err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		defer func() { _ = sqlDB.Close() }()
		var name string
		if err := db.Raw("SELECT current_database()").Scan(&name).Error; err != nil {
			return err
		}
		if !strings.Contains(strings.ToLower(name), "test") {
			return fmt.Errorf("refusing to change database %q", name)
		}
		return db.Exec(statement).Error
	}
	suffix := make([]byte, 4)
	_, err := rand.Read(suffix)
	require.NoError(t, err)
	schema := "meshregion_" + hex.EncodeToString(suffix)
	require.NoError(t, admin(`CREATE SCHEMA "`+schema+`"`))
	t.Cleanup(func() { _ = admin(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`) })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), quiet)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(model.MeshModels(), &model.Node{}, &model.ForwardNode{})...))
	for i := 1; i <= 8; i++ {
		host := fmt.Sprintf("n%d.example.com", i)
		require.NoError(t, db.Create(&model.Node{ID: uint(i), Name: host[:2], Host: host}).Error)
	}
	return db
}

// together runs the functions at the same time and answers their errors.
func together(fns ...func() error) []error {
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = fn()
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

// requireDistinctAddresses checks that no two members, devices or unused
// full node credentials hold one address.
func requireDistinctAddresses(t *testing.T, db *gorm.DB) int {
	t.Helper()
	seen := map[string]string{}
	add := func(owner, address string) {
		if address == "" {
			return
		}
		previous, taken := seen[address]
		require.False(t, taken, "%s holds %s of %s", owner, address, previous)
		seen[address] = owner
	}
	var members []model.MeshMember
	require.NoError(t, db.Find(&members).Error)
	for _, member := range members {
		add(fmt.Sprintf("member %d", member.ID), member.Address)
	}
	var devices []model.MeshDevice
	require.NoError(t, db.Find(&devices).Error)
	for _, device := range devices {
		if device.Address != nil {
			add(fmt.Sprintf("device %d", device.ID), *device.Address)
		}
	}
	var reservations []model.MeshFullNode
	require.NoError(t, db.Where("device_id IS NULL").Find(&reservations).Error)
	for _, reservation := range reservations {
		add(fmt.Sprintf("credential %d", reservation.EnrollmentID), reservation.Address)
	}
	return len(seen)
}

func TestPostgresConcurrentAddresses(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	eu, err := CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.152.0.0/24", Enabled: true}, now)
	require.NoError(t, err)
	_, err = CreateRegion(ctx, db, RegionInput{Code: "as", Name: "as", CIDR: "10.152.1.0/24", Enabled: true}, now)
	require.NoError(t, err)

	var fns []func() error
	for i := 0; i < 12; i++ {
		fns = append(fns, func() error {
			_, err := CreateDevice(ctx, db, eu.ID, DeviceInput{Hostname: fmt.Sprintf("phone-%d", i)}, now)
			return err
		})
	}
	for i := 0; i < 4; i++ {
		fns = append(fns, func() error {
			_, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: eu.ID, Hostname: fmt.Sprintf("router-%d", i), Role: RoleFull, Address: fmt.Sprintf("10.152.0.%d", 121+i)}, nil, now)
			return err
		})
	}
	for id := uint32(1); id <= 4; id++ {
		fns = append(fns, func() error {
			_, err := AddNode(ctx, db, eu.ID, proxy(id), id == 1, now)
			return err
		})
	}
	for i, err := range together(fns...) {
		require.NoError(t, err, "change %d", i)
	}
	// 12 devices, 4 credentials keeping the addresses they name (a
	// credential without one keeps none), 4 nodes and the boundary node's
	// address in the other region.
	require.Equal(t, 21, requireDistinctAddresses(t, db))
}

// Boundary nodes join every region, and moves lock two regions: such
// changes at the same time lock the regions in one order and finish.
func TestPostgresBoundaryNodesTogether(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	var regions []model.MeshRegion
	for i, code := range []string{"eu", "as", "us"} {
		created, err := CreateRegion(ctx, db, RegionInput{Code: code, Name: code, CIDR: fmt.Sprintf("10.152.%d.0/24", i), Enabled: true}, now)
		require.NoError(t, err)
		regions = append(regions, created)
	}
	retries := changeRetries.Load()
	for round := 0; round < 3; round++ {
		var fns []func() error
		for i := 0; i < 3; i++ {
			id := uint32(i + 1)
			target := regions[len(regions)-1-i]
			fns = append(fns, func() error {
				_, err := AddNode(ctx, db, target.ID, proxy(id), true, now)
				return err
			})
			fns = append(fns, func() error {
				_, err := CreateDevice(ctx, db, target.ID, DeviceInput{Hostname: fmt.Sprintf("d-%d-%d", round, i)}, now)
				return err
			})
		}
		for i, err := range together(fns...) {
			require.NoError(t, err, "round %d change %d", round, i)
		}
		requireDistinctAddresses(t, db)
		var homes []model.MeshMember
		require.NoError(t, db.Where("home = ?", true).Find(&homes).Error)
		for _, home := range homes {
			require.NoError(t, RemoveNode(ctx, db, home.ID))
		}
	}
	require.Equal(t, retries, changeRetries.Load(), "no change collided: the regions are locked in one order")
}

// A range changes only while the region has no devices: a range change
// and devices joining at the same time either refuse the change or give
// the devices addresses of the new range, never of the old one.
func TestPostgresRangeChangeTogether(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	for round := 0; round < 5; round++ {
		created, err := CreateRegion(ctx, db, RegionInput{Code: fmt.Sprintf("r%d", round), Name: fmt.Sprintf("r%d", round), CIDR: fmt.Sprintf("10.152.%d.0/24", 2*round), Enabled: true}, now)
		require.NoError(t, err)
		fns := []func() error{func() error {
			_, err := UpdateRegion(ctx, db, created.ID, RegionInput{Code: created.Code, Name: created.Name, CIDR: fmt.Sprintf("10.152.%d.0/24", 2*round+1), Enabled: true}, now)
			if errors.Is(err, ErrConflict) {
				return nil
			}
			return err
		}}
		for i := 0; i < 6; i++ {
			fns = append(fns, func() error {
				_, err := CreateDevice(ctx, db, created.ID, DeviceInput{Hostname: fmt.Sprintf("d-%d-%d", round, i)}, now)
				return err
			})
		}
		for i, err := range together(fns...) {
			require.NoError(t, err, "round %d change %d", round, i)
		}
		var region model.MeshRegion
		require.NoError(t, db.First(&region, created.ID).Error)
		prefix := netip.MustParsePrefix(region.CIDR)
		var devices []model.MeshDevice
		require.NoError(t, db.Where("region_id = ?", region.ID).Find(&devices).Error)
		require.Len(t, devices, 6)
		for _, device := range devices {
			require.True(t, prefix.Contains(netip.MustParseAddr(*device.Address)), "%s outside %s", *device.Address, region.CIDR)
		}
	}
	requireDistinctAddresses(t, db)
}

// A full node's subnets and a new region over them, at the same time:
// exactly one of the two is accepted.
func TestPostgresPlanChangesTogether(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	home, err := CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.152.0.0/24", Enabled: true}, now)
	require.NoError(t, err)
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: home.ID, Hostname: "router", Role: RoleFull}, nil, now)
	require.NoError(t, err)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: strings.Repeat("r", 32)}, now)
	require.NoError(t, err)
	for round := 0; round < 5; round++ {
		subnet := fmt.Sprintf("192.168.%d.0/24", round)
		errs := together(func() error {
			_, err := SetDeviceSubnets(ctx, db, enrolled.DeviceID, []string{subnet}, now)
			return err
		}, func() error {
			_, err := CreateRegion(ctx, db, RegionInput{Code: fmt.Sprintf("lan%d", round), Name: fmt.Sprintf("lan%d", round), CIDR: subnet}, now)
			return err
		})
		accepted := 0
		for _, err := range errs {
			if err == nil {
				accepted++
			} else {
				require.ErrorIs(t, err, ErrConflict)
			}
		}
		require.Equal(t, 1, accepted, "round %d: %v", round, errs)
	}
}

// Requests, approvals, direct moves and deletes of the same devices at the
// same time: each either happens or is refused as a conflict, and every
// device keeps one address of its region.
func TestPostgresChangesTogether(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	from, err := CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.152.0.0/24", Enabled: true}, now)
	require.NoError(t, err)
	to, err := CreateRegion(ctx, db, RegionInput{Code: "as", Name: "as", CIDR: "10.152.1.0/24", Enabled: true}, now)
	require.NoError(t, err)
	tolerated := func(err error) error {
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalid) {
			return nil
		}
		return err
	}
	for round := 0; round < 4; round++ {
		var ids []uint
		for i := 0; i < 3; i++ {
			device, err := CreateDevice(ctx, db, from.ID, DeviceInput{Hostname: fmt.Sprintf("c-%d-%d", round, i)}, now)
			require.NoError(t, err)
			ids = append(ids, device.ID)
		}
		pending, err := RequestChange(ctx, db, ChangeInput{DeviceID: ids[0], ToRegionID: to.ID}, model.MeshChangeAdmin, nil, now)
		require.NoError(t, err)
		errs := together(
			func() error {
				_, err := RequestChange(ctx, db, ChangeInput{DeviceID: ids[1], ToRegionID: to.ID}, model.MeshChangeAdmin, nil, now)
				return tolerated(err)
			},
			func() error {
				_, err := MoveDevice(ctx, db, ids[1], to.ID, nil, now)
				return tolerated(err)
			},
			func() error {
				_, err := ApproveChange(ctx, db, pending.ID, "", nil, now)
				return tolerated(err)
			},
			func() error { return tolerated(DeleteDevice(ctx, db, ids[0], now)) },
			func() error {
				_, err := AssignDeviceAddress(ctx, db, ids[2], now)
				return tolerated(err)
			},
		)
		for i, err := range errs {
			require.NoError(t, err, "round %d change %d", round, i)
		}
		requireDistinctAddresses(t, db)
	}
}
