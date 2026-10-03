package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// manifestPath is where seed writes the World the replayer reads.
const manifestPath = "/work/seed-manifest.json"

// personaPassword is the password of every seeded persona. It protects
// nothing: the data set is synthetic and the stack has no route out.
const personaPassword = "Staging-Persona-2026"

// Persona is a seeded account the replayer logs in as.
type Persona struct {
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
	ID       uint   `json:"id"`
}

// World is what the seeder created: personas and named id sets the
// request generators pick path parameters and bodies from.
type World struct {
	Seed     int64               `json:"seed"`
	Personas map[string]Persona  `json:"personas"`
	IDs      map[string][]uint   `json:"ids"`
	Strings  map[string][]string `json:"strings"`
	// Base is the fixed clock of the data set: seeded times are offsets
	// from it, so two seeds of the same seed number are identical.
	Base time.Time `json:"base"`
}

func newWorld(seed int64) *World {
	return &World{
		Seed: seed, Personas: map[string]Persona{}, IDs: map[string][]uint{}, Strings: map[string][]string{},
		Base: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
}

// Add records ids under a key.
func (w *World) Add(key string, ids ...uint) { w.IDs[key] = append(w.IDs[key], ids...) }

// AddString records strings under a key.
func (w *World) AddString(key string, values ...string) {
	w.Strings[key] = append(w.Strings[key], values...)
}

// ID returns the i-th id of a key (wrapping), or 0 when the key is empty;
// generators should treat 0 as "missing" and use Missing instead.
func (w *World) ID(key string, i int) uint {
	ids := w.IDs[key]
	if len(ids) == 0 {
		return 0
	}
	return ids[((i%len(ids))+len(ids))%len(ids)]
}

// Str returns the i-th string of a key (wrapping), or "".
func (w *World) Str(key string, i int) string {
	values := w.Strings[key]
	if len(values) == 0 {
		return ""
	}
	return values[((i%len(values))+len(values))%len(values)]
}

// Persona returns a persona's id.
func (w *World) PersonaID(key string) uint { return w.Personas[key].ID }

// Missing is an id no seeded row has.
const Missing = 987654

func loadWorld(path string) (*World, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the seed manifest path is fixed
	if err != nil {
		return nil, fmt.Errorf("read the seed manifest (run seed first): %w", err)
	}
	var world World
	if err := json.Unmarshal(data, &world); err != nil {
		return nil, err
	}
	return &world, nil
}

// Seeder is the context of one seed step.
type Seeder struct {
	DB    *gorm.DB
	World *World
	Rand  *rand.Rand
	Scale int
}

// At is a fixed time of the data set: Base plus the offset.
func (s *Seeder) At(offset time.Duration) time.Time { return s.World.Base.Add(offset) }

// Days is a helper for day offsets.
func Days(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// Create inserts rows and fails loudly.
func (s *Seeder) Create(value any) {
	if err := s.DB.Create(value).Error; err != nil {
		panic(fmt.Errorf("seed: create %T: %w", value, err))
	}
}

type seedStep struct {
	Order int
	Name  string
	Run   func(*Seeder)
}

var seedSteps []seedStep

// registerSeed adds a seed step; steps run by Order (users and plans are
// 10 and 20, so every other step may use them).
func registerSeed(order int, name string, run func(*Seeder)) {
	seedSteps = append(seedSteps, seedStep{Order: order, Name: name, Run: run})
}

type seedOptions struct {
	Host, User, Password, Database string
	Seed                           int64
	Scale                          int
}

func openDatabase(host, user, password, database string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC", host, user, password, database)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func seed(ctx context.Context, options seedOptions) (err error) {
	db, err := openDatabase(options.Host, options.User, options.Password, options.Database)
	if err != nil {
		return err
	}
	var users int64
	if err := db.WithContext(ctx).Model(&model.User{}).Count(&users).Error; err != nil {
		return fmt.Errorf("the schema is missing (start Control first): %w", err)
	}
	if users > 1 {
		return errors.New("the database already holds more than the bootstrap administrator: seed only a fresh stack")
	}
	if options.Scale < 1 {
		options.Scale = 1
	}
	world := newWorld(options.Seed)
	sort.SliceStable(seedSteps, func(i, j int) bool { return seedSteps[i].Order < seedSteps[j].Order })
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, step := range seedSteps {
			// Each step gets its own random stream, so adding a step does
			// not change the data of the others.
			seeder := &Seeder{DB: tx, World: world, Rand: rand.New(rand.NewSource(options.Seed + int64(step.Order))), Scale: options.Scale} // #nosec G404 -- the synthetic data set must be reproducible from its seed
			step.Run(seeder)
			fmt.Printf("seeded %s\n", step.Name)
		}
		return syncSequences(tx)
	})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(world, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		return err
	}
	fmt.Printf("seed %d: %d personas, %d id sets written to %s\n", options.Seed, len(world.Personas), len(world.IDs), manifestPath)
	return nil
}

// syncSequences moves every serial sequence past explicitly seeded ids.
func syncSequences(tx *gorm.DB) error {
	var rows []struct {
		Table  string
		Column string
	}
	if err := tx.Raw(`SELECT c.table_name AS "table", c.column_name AS "column"
		FROM information_schema.columns c
		WHERE c.table_schema = 'public' AND c.column_default LIKE 'nextval(%'`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		statement := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%[1]q', '%[2]s'), GREATEST((SELECT COALESCE(MAX(%[2]q), 0) FROM %[1]q), 1))`, row.Table, row.Column)
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("sync sequence %s.%s: %w", row.Table, row.Column, err)
		}
	}
	return nil
}

func ptr[T any](value T) *T { return &value }

func init() {
	registerSeed(10, "node groups, plans and users", seedUsersAndPlans)
}

// seedUsersAndPlans writes node groups, plans and the member base: three
// administrators (the bootstrap super administrator, a second one and a
// staff member), members with plans, banned and expired members and
// members without any data.
func seedUsersAndPlans(s *Seeder) {
	groups := []model.NodeGroup{
		{Name: "Standard", Sort: 1}, {Name: "Premium", Sort: 2}, {Name: "香港 HK", Sort: 3}, {Name: "日本 JP", Sort: 4},
	}
	for i := range groups {
		groups[i].CreatedAt, groups[i].UpdatedAt = s.At(Days(-200)), s.At(Days(-200))
	}
	s.Create(&groups)
	for _, group := range groups {
		s.World.Add("node_group", group.ID)
	}

	type planShape struct {
		name     string
		group    int
		gb       int64
		show     int
		renew    int
		month    *int64
		quarter  *int64
		year     *int64
		onetime  *int64
		reset    *int64
		capacity *int
	}
	shapes := []planShape{
		{name: "Starter 入门", group: 0, gb: 50, show: 1, renew: 1, month: ptr[int64](990), quarter: ptr[int64](2790), year: ptr[int64](9900)},
		{name: "Standard 标准", group: 0, gb: 200, show: 1, renew: 1, month: ptr[int64](1990), year: ptr[int64](19900), reset: ptr[int64](500)},
		{name: "Premium 高级", group: 1, gb: 1000, show: 1, renew: 1, month: ptr[int64](4990), quarter: ptr[int64](13990), year: ptr[int64](49900)},
		{name: "Lifetime 永久", group: 1, gb: 100, show: 1, renew: 0, onetime: ptr[int64](29900)},
		{name: "Hidden legacy plan", group: 0, gb: 30, show: 0, renew: 0, month: ptr[int64](500)},
		{name: "日本 Japan", group: 3, gb: 300, show: 1, renew: 1, month: ptr[int64](2990), capacity: ptr(3)},
		{name: "Sold out 售罄", group: 2, gb: 500, show: 1, renew: 1, month: ptr[int64](3990), capacity: ptr(0)},
		{name: "Free trial", group: 0, gb: 5, show: 0, renew: 0, month: ptr[int64](0)},
	}
	plans := make([]model.Plan, len(shapes))
	for i, shape := range shapes {
		content := fmt.Sprintf("<p>%s: %d GB, synthetic staging plan</p>", shape.name, shape.gb)
		plans[i] = model.Plan{
			GroupID: groups[shape.group].ID, TransferEnable: shape.gb, Name: shape.name, Content: &content, Show: shape.show,
			Sort: ptr(i + 1), Renew: shape.renew, MonthPrice: shape.month, QuarterPrice: shape.quarter, YearPrice: shape.year,
			OnetimePrice: shape.onetime, ResetPrice: shape.reset, CapacityLimit: shape.capacity,
			DeviceLimit: ptr(3 + i%3), SpeedLimit: ptr[int64](int64(100 * (1 + i%4))),
			CreatedAt: s.At(Days(-180 + i)), UpdatedAt: s.At(Days(-180 + i)),
		}
	}
	s.Create(&plans)
	for i, plan := range plans {
		s.World.Add("plan", plan.ID)
		if shapes[i].show == 1 {
			s.World.Add("plan.visible", plan.ID)
		} else {
			s.World.Add("plan.hidden", plan.ID)
		}
	}

	// The bootstrap super administrator (Control created it from
	// ANIX_CONTROL_ADMIN_EMAIL).
	var root model.User
	if err := s.DB.Where("is_admin = 1").Order("id").First(&root).Error; err != nil {
		panic(fmt.Errorf("seed: bootstrap administrator: %w", err))
	}
	s.World.Personas[Admin] = Persona{Email: root.Email, ID: root.ID}

	hash, err := bcrypt.GenerateFromPassword([]byte(personaPassword), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	now := s.World.Base
	future := now.Add(Days(3650)).Unix()
	past := now.Add(Days(-30)).Unix()
	makeUser := func(email string, mutate func(*model.User)) *model.User {
		user := &model.User{
			Email: email, Password: string(hash),
			Token: fmt.Sprintf("%032x", s.Rand.Uint64())[:32], UUID: fakeUUID(s.Rand),
			TransferEnable: int64(1+s.Rand.Intn(500)) << 30, CreatedAt: s.At(Days(-150 + s.Rand.Intn(140))),
		}
		user.UpdatedAt = user.CreatedAt
		if mutate != nil {
			mutate(user)
		}
		s.Create(user)
		return user
	}
	persona := func(key string, user *model.User) {
		s.World.Personas[key] = Persona{Email: user.Email, Password: personaPassword, ID: user.ID}
	}
	persona(Admin2, makeUser("ops@staging.example.com", func(u *model.User) { u.IsAdmin = 1 }))
	s.World.Add("user.admin", s.World.Personas[Admin2].ID)
	persona(Staff, makeUser("staff@staging.example.com", func(u *model.User) { u.IsAdmin, u.IsStaff = 1, 1 }))
	s.World.Add("user.admin", s.World.Personas[Staff].ID)

	members := 60 * s.Scale
	for i := 0; i < members; i++ {
		plan := plans[s.Rand.Intn(4)]
		user := makeUser(fmt.Sprintf("member%03d@example.com", i+1), func(u *model.User) {
			u.PlanID, u.GroupID, u.ExpiredAt = &plan.ID, &plan.GroupID, ptr(future-int64(s.Rand.Intn(300))*86400)
			u.U, u.D = int64(s.Rand.Intn(40))<<30, int64(s.Rand.Intn(120))<<30
			u.Balance, u.CommissionBalance = int64(s.Rand.Intn(5000)), int64(s.Rand.Intn(3000))
			u.LastLoginAt = ptr(now.Add(-time.Duration(s.Rand.Intn(72)) * time.Hour).Unix())
			if i%7 == 0 {
				u.TelegramID = ptr(int64(700000000 + i))
			}
			if i%5 == 0 {
				u.Discount = ptr(10 + i%20)
			}
			if i%9 == 0 {
				remark := fmt.Sprintf("synthetic remark %d", i)
				u.RemarkContent = &remark
			}
		})
		s.World.Add("user.member", user.ID)
		if i == 0 {
			persona(User, user)
		}
		if i == 1 {
			persona(User2, user)
		}
	}
	for i := 0; i < 8*s.Scale; i++ {
		user := makeUser(fmt.Sprintf("banned%02d@example.com", i+1), func(u *model.User) {
			u.Banned = 1
			u.PlanID = &plans[i%3].ID
			u.ExpiredAt = ptr(future)
		})
		s.World.Add("user.banned", user.ID)
		if i == 0 {
			persona(Banned, user)
		}
	}
	for i := 0; i < 12*s.Scale; i++ {
		user := makeUser(fmt.Sprintf("expired%02d@example.org", i+1), func(u *model.User) {
			u.PlanID = &plans[i%4].ID
			u.ExpiredAt = ptr(past - int64(i)*86400)
			u.U, u.D = int64(i)<<30, int64(2*i)<<30
		})
		s.World.Add("user.expired", user.ID)
		if i == 0 {
			persona(Expired, user)
		}
	}
	for i := 0; i < 5*s.Scale; i++ {
		user := makeUser(fmt.Sprintf("fresh%02d@example.net", i+1), func(u *model.User) { u.TransferEnable = 0 })
		s.World.Add("user.fresh", user.ID)
		if i == 0 {
			persona(Fresh, user)
		}
	}
	// Invitation tree: members 3..20 were invited by the user persona or
	// user2, so commissions and invite statistics have something to show.
	memberIDs := s.World.IDs["user.member"]
	for i := 2; i < 20 && i < len(memberIDs); i++ {
		inviter := s.World.Personas[User].ID
		if i%3 == 0 {
			inviter = s.World.Personas[User2].ID
		}
		if err := s.DB.Model(&model.User{}).Where("id = ?", memberIDs[i]).Update("invite_user_id", inviter).Error; err != nil {
			panic(err)
		}
		s.World.Add("user.invited", memberIDs[i])
	}
}

func fakeUUID(r *rand.Rand) string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(r.Intn(256)) // #nosec G115 -- Intn(256) is always within a byte
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
