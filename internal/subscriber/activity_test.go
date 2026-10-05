package subscriber

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// forEachActivityDatabase runs body on SQLite and, with
// ANIX_TEST_POSTGRES_DSN, on a throwaway PostgreSQL schema, each with
// subscribers 1 to 3 and the activity table.
func forEachActivityDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	seed := func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(&model.UserActivity{}))
		for id := uint(1); id <= 3; id++ {
			require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("u%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id)}).Error)
		}
	}
	t.Run("sqlite", func(t *testing.T) {
		db := openDB(t)
		seed(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openPostgresDB(t)
		seed(t, db)
		body(t, db)
	})
}

func lastOnline(t *testing.T, db *gorm.DB, ids ...uint) map[uint]int64 {
	t.Helper()
	seen, err := LastOnline(db, ids)
	require.NoError(t, err)
	return seen
}

// A user is written once per interval however many reports name them: the
// stored time is the first report's of the interval, and the next interval's
// report moves it.
func TestRecordOnlineWritesOncePerInterval(t *testing.T) {
	forEachActivityDatabase(t, func(t *testing.T, db *gorm.DB) {
		start := time.Unix(1_800_000_000, 0)
		RecordOnline(db, []uint{1, 2}, start)
		require.Equal(t, map[uint]int64{1: start.Unix(), 2: start.Unix()}, lastOnline(t, db, 1, 2, 3))

		// Within the interval nothing is written for 1 and 2; 3 is new.
		RecordOnline(db, []uint{1, 2, 3}, start.Add(30*time.Second))
		require.Equal(t, map[uint]int64{1: start.Unix(), 2: start.Unix(), 3: start.Add(30 * time.Second).Unix()}, lastOnline(t, db, 1, 2, 3))

		// At the interval the user's time moves forward.
		RecordOnline(db, []uint{1}, start.Add(ActivityWriteInterval))
		require.Equal(t, start.Add(ActivityWriteInterval).Unix(), lastOnline(t, db, 1)[1])
		require.Equal(t, start.Unix(), lastOnline(t, db, 2)[2])
	})
}

// A stored time only moves forward: a report from a process that was slower
// (or a clock that is behind) cannot move it back.
func TestRecordOnlineNeverMovesATimeBack(t *testing.T) {
	forEachActivityDatabase(t, func(t *testing.T, db *gorm.DB) {
		at := time.Unix(1_800_000_000, 0)
		require.NoError(t, upsertActivity(db, []uint{1}, at.Unix()))
		require.NoError(t, upsertActivity(db, []uint{1}, at.Add(-time.Hour).Unix()))
		require.Equal(t, at.Unix(), lastOnline(t, db, 1)[1])
		require.NoError(t, upsertActivity(db, []uint{1}, at.Add(time.Hour).Unix()))
		require.Equal(t, at.Add(time.Hour).Unix(), lastOnline(t, db, 1)[1])
	})
}

// A report that names a user id that is not a subscriber leaves no row, and
// neither does id zero.
func TestRecordOnlineIgnoresWhoIsNoSubscriber(t *testing.T) {
	forEachActivityDatabase(t, func(t *testing.T, db *gorm.DB) {
		RecordOnline(db, []uint{0, 1, 99, 100}, time.Unix(1_800_000_000, 0))
		var rows []model.UserActivity
		require.NoError(t, db.Order("user_id").Find(&rows).Error)
		require.Equal(t, []model.UserActivity{{UserID: 1, LastOnlineAt: 1_800_000_000}}, rows)
	})
}

// A report with more users than one statement takes writes them all.
func TestRecordOnlineWritesLargeBatches(t *testing.T) {
	forEachActivityDatabase(t, func(t *testing.T, db *gorm.DB) {
		const users = 2*activityBatch + 7
		rows := make([]model.User, 0, users)
		ids := make([]uint, 0, users)
		for id := uint(1000); id < 1000+users; id++ {
			rows = append(rows, model.User{ID: id, Email: fmt.Sprintf("bulk%d@example.test", id), Token: fmt.Sprintf("bt%d", id), UUID: fmt.Sprintf("bu%d", id)})
			ids = append(ids, id)
		}
		require.NoError(t, db.CreateInBatches(rows, 200).Error)
		RecordOnline(db, ids, time.Unix(1_800_000_000, 0))
		var count int64
		require.NoError(t, db.Model(&model.UserActivity{}).Count(&count).Error)
		require.EqualValues(t, users, count)
	})
}

// Each database has its own throttle: another database in the same process
// is written even though a user was just written on the first.
func TestRecordOnlineThrottlesPerDatabase(t *testing.T) {
	first, second := openDB(t), openDB(t)
	for _, db := range []*gorm.DB{first, second} {
		require.NoError(t, db.AutoMigrate(&model.UserActivity{}))
		require.NoError(t, db.Create(&model.User{ID: 1, Email: "u1@example.test", Token: "t1", UUID: "u1"}).Error)
	}
	at := time.Unix(1_800_000_000, 0)
	RecordOnline(first, []uint{1}, at)
	RecordOnline(second, []uint{1}, at)
	require.Equal(t, at.Unix(), lastOnline(t, first, 1)[1])
	require.Equal(t, at.Unix(), lastOnline(t, second, 1)[1])
}

// Recording is best effort: a database without the table, or no database,
// does not fail or panic.
func TestRecordOnlineIsBestEffort(t *testing.T) {
	db := openDB(t)
	RecordOnline(db, []uint{1}, time.Unix(1_800_000_000, 0))
	RecordOnline(nil, []uint{1}, time.Now())
	RecordOnline(db, nil, time.Now())
}

// A deleted user's time goes with them; a database without the table is left
// alone.
func TestForgetActivity(t *testing.T) {
	forEachActivityDatabase(t, func(t *testing.T, db *gorm.DB) {
		RecordOnline(db, []uint{1, 2}, time.Unix(1_800_000_000, 0))
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return ForgetActivityTx(tx, 1) }))
		require.Equal(t, map[uint]int64{2: 1_800_000_000}, lastOnline(t, db, 1, 2))
	})
	bare := openDB(t)
	require.NoError(t, bare.Transaction(func(tx *gorm.DB) error { return ForgetActivityTx(tx, 1) }))
}

// The backfill statement of docs/reference/traffic-stats-operations.md runs
// on both databases and seeds the table from the traffic log, for the users
// that exist, without moving a later time back.
func TestDocumentedBackfillSeedsTheTableFromTheTrafficLog(t *testing.T) {
	doc, err := os.ReadFile("../../docs/reference/traffic-stats-operations.md")
	require.NoError(t, err)
	start := strings.Index(string(doc), "INSERT INTO v4_kernel_user_activity")
	require.GreaterOrEqual(t, start, 0, "the backfill statement is documented")
	statement := string(doc)[start:]
	statement = statement[:strings.Index(statement, "```")]

	forEachActivityDatabase(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(&model.TrafficLog{}))
		require.NoError(t, db.Create(&[]model.TrafficLog{
			{UserID: 1, ServerID: 1, LogAt: 1_700_000_000}, {UserID: 1, ServerID: 1, LogAt: 1_700_000_500},
			{UserID: 2, ServerID: 1, LogAt: 1_700_000_100}, {UserID: 99, ServerID: 1, LogAt: 1_700_000_900},
		}).Error)
		// User 2 was seen later than the log says.
		require.NoError(t, upsertActivity(db, []uint{2}, 1_800_000_000))
		require.NoError(t, db.Exec(statement).Error)
		require.Equal(t, map[uint]int64{1: 1_700_000_500, 2: 1_800_000_000}, lastOnline(t, db, 1, 2, 3, 99))
	})
}
