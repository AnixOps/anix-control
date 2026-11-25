package meshregion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
			_, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: eu.ID, Hostname: fmt.Sprintf("router-%d", i), Role: RoleFull}, nil, now)
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
	// 12 devices, 4 credentials, 4 nodes and the boundary node's address
	// in the other region.
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
}
