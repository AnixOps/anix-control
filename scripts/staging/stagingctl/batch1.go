package main

import (
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// Batch 1: knowledge + ticket.

func init() {
	registerSeed(60, "knowledge articles", seedKnowledge)
	registerSeed(61, "tickets and replies", seedTickets)

	registerTables("knowledge", TableSpec{Table: "v2_knowledge", Ignore: timestamps()})
	registerTables("ticket",
		TableSpec{Table: "v2_ticket", Ignore: timestamps()},
		TableSpec{Table: "v2_ticket_message", Ignore: timestamps()},
	)

	registerSpecs(
		RouteSpec{RouteID: "knowledge.article.list", Reads: func(w *World) []Req {
			path := "/api/v2/user/knowledge"
			return []Req{
				{Persona: User, Path: path, Label: "visible articles"},
				{Persona: Fresh, Path: path, Label: "member without data"},
				{Persona: User2, Path: path, Query: q("language", "en-US"), Label: "ignored language filter"},
				{Persona: Expired, Path: path, Label: "expired member"},
				{Persona: Banned, Path: path, Label: "banned member"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "knowledge.user.knowledge.id.get", Reads: func(w *World) []Req {
			var reqs []Req
			for i := 0; i < 6; i++ {
				reqs = append(reqs, Req{Persona: User, Path: fill("/api/v2/user/knowledge/:id", w.ID("knowledge.visible", i*7)), Label: "visible article"})
			}
			return append(reqs,
				Req{Persona: User, Path: fill("/api/v2/user/knowledge/:id", w.ID("knowledge.hidden", 0)), Label: "hidden article"},
				Req{Persona: User2, Path: fill("/api/v2/user/knowledge/:id", w.ID("knowledge.hidden", 3)), Label: "hidden article"},
				Req{Persona: User, Path: fill("/api/v2/user/knowledge/:id", Missing), Label: "not found"},
				Req{Persona: User, Path: "/api/v2/user/knowledge/abc", Label: "invalid id"},
				Req{Persona: User, Path: "/api/v2/user/knowledge/-1", Label: "negative id"},
				Req{Persona: Anon, Path: fill("/api/v2/user/knowledge/:id", w.ID("knowledge.visible", 0)), Label: "anonymous"},
			)
		}},
		RouteSpec{RouteID: "knowledge.admin.knowledge.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/knowledge"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every article"},
				{Persona: Admin2, Path: path, Query: q("page", "2", "page_size", "10"), Label: "ignored pagination"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "knowledge.admin.knowledge.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/knowledge"
			created := []string{"data.data.created_at", "data.data.updated_at", "data.created_at", "data.updated_at"}
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"category": "帮助", "title": "Staging 新文章", "body": "<p>正文</p>", "sort": 3, "show": 1}, Mask: created, Label: "create"},
				{Persona: Admin, Path: path, Body: map[string]any{"category": "Help", "title": "English article", "body": "body", "language": "en-US"}, Mask: created, Label: "create with unknown field"},
				{Persona: Staff, Path: path, Body: map[string]any{"title": "defaults", "body": "b"}, Mask: created, Label: "create with defaults"},
				{Persona: Admin, Path: path, Body: map[string]any{"body": "no title"}, Label: "missing title"},
				{Persona: Admin, Path: path, Body: `{"title":`, Label: "invalid JSON"},
				{Persona: User, Path: path, Body: map[string]any{"title": "x", "body": "y"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "knowledge.admin.knowledge.id.put", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", w.ID("knowledge.visible", 1)), Body: map[string]any{"title": "renamed 改名", "show": 0, "sort": 7}, Label: "partial update"},
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", w.ID("knowledge.hidden", 1)), Body: map[string]any{"show": 1, "category": "公告"}, Label: "publish hidden"},
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", w.ID("knowledge.visible", 2)), Body: map[string]any{}, Label: "empty update"},
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", w.ID("knowledge.visible", 3)), Body: map[string]any{"show": 2}, Label: "show out of range"},
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", Missing), Body: map[string]any{"title": "x"}, Label: "not found"},
				{Persona: Admin, Path: "/api/v2/admin/knowledge/abc", Body: map[string]any{"title": "x"}, Label: "invalid id"},
			}
		}},
		RouteSpec{RouteID: "knowledge.admin.knowledge.id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", w.ID("knowledge.visible", 5)), Label: "delete"},
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", w.ID("knowledge.visible", 5)), Label: "delete again"},
				{Persona: Admin, Path: fill("/api/v2/admin/knowledge/:id", Missing), Label: "not found"},
				{Persona: Admin, Path: "/api/v2/admin/knowledge/abc", Label: "invalid id"},
			}
		}},

		RouteSpec{RouteID: "ticket.user.ticket.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/ticket"
			return []Req{
				{Persona: User, Path: path, Label: "own tickets"},
				{Persona: User2, Path: path, Label: "own tickets"},
				{Persona: Fresh, Path: path, Label: "no tickets"},
				{Persona: Banned, Path: path, Label: "banned member"},
				{Persona: Expired, Path: path, Label: "expired member"},
				{Persona: User, Path: path, Query: q("page", "2", "status", "0"), Label: "ignored filters"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "ticket.user.ticket.id.get", Reads: func(w *World) []Req {
			return []Req{
				{Persona: User, Path: fill("/api/v2/user/ticket/:id", w.ID("ticket.user.open", 0)), Label: "own open ticket with replies"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id", w.ID("ticket.user.closed", 0)), Label: "own closed ticket"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id", w.ID("ticket.user.answered", 0)), Label: "own answered ticket"},
				{Persona: User2, Path: fill("/api/v2/user/ticket/:id", w.ID("ticket.user2", 0)), Label: "own ticket"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id", w.ID("ticket.user2", 0)), Label: "another member's ticket"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id", Missing), Label: "not found"},
				{Persona: User, Path: "/api/v2/user/ticket/x", Label: "invalid id"},
			}
		}},
		RouteSpec{RouteID: "ticket.user.ticket.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/ticket"
			created := []string{"data.created_at", "data.updated_at"}
			return []Req{
				{Persona: User, Path: path, Body: map[string]any{"subject": "无法连接 cannot connect", "level": 2, "message": "节点超时 timeout"}, Mask: created, Label: "create"},
				{Persona: Fresh, Path: path, Body: map[string]any{"subject": "first ticket", "level": 0, "message": "hello"}, Mask: created, Label: "create (first)"},
				{Persona: User, Path: path, Body: map[string]any{"message": "no subject"}, Label: "missing subject"},
				{Persona: User, Path: path, Body: map[string]any{"subject": "no message"}, Label: "missing message"},
				{Persona: User, Path: path, Body: map[string]any{"subject": "bad level", "level": 9, "message": "m"}, Mask: created, Label: "level out of range"},
				{Persona: Anon, Path: path, Body: map[string]any{"subject": "s", "message": "m"}, Label: "anonymous"},
				{Persona: Banned, Path: path, Body: map[string]any{"subject": "banned", "message": "m"}, Mask: created, Label: "banned member"},
			}
		}},
		RouteSpec{RouteID: "ticket.user.ticket.id.reply.post", Writes: func(w *World) []Req {
			return []Req{
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/reply", w.ID("ticket.user.answered", 0)), Body: map[string]any{"message": "still broken"}, Label: "reply reopens"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/reply", w.ID("ticket.user.open", 1)), Body: map[string]any{"message": "more details"}, Label: "reply"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/reply", w.ID("ticket.user.closed", 0)), Body: map[string]any{"message": "closed?"}, Label: "closed ticket"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/reply", w.ID("ticket.user2", 0)), Body: map[string]any{"message": "not mine"}, Label: "another member's ticket"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/reply", w.ID("ticket.user.open", 0)), Body: map[string]any{}, Label: "missing message"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/reply", Missing), Body: map[string]any{"message": "m"}, Label: "not found"},
			}
		}},
		RouteSpec{RouteID: "ticket.user.ticket.id.close.post", Writes: func(w *World) []Req {
			return []Req{
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/close", w.ID("ticket.user.open", 2)), Label: "close"},
				{Persona: User, Path: fill("/api/v2/user/ticket/:id/close", w.ID("ticket.user.closed", 0)), Label: "close closed"},
				{Persona: User2, Path: fill("/api/v2/user/ticket/:id/close", w.ID("ticket.user.open", 0)), Label: "another member's ticket"},
				{Persona: User, Path: "/api/v2/user/ticket/x/close", Label: "invalid id"},
			}
		}},
		RouteSpec{RouteID: "ticket.admin.ticket.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/ticket"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every ticket"},
				{Persona: Staff, Path: path, Label: "staff"},
				{Persona: Admin2, Path: path, Query: q("status", "1", "page", "3"), Label: "ignored filters"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "ticket.admin.ticket.reply.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/ticket/reply"
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"ticket_id": w.ID("ticket.user.open", 3), "message": "已处理 handled"}, Label: "reply answers"},
				{Persona: Staff, Path: path, Body: map[string]any{"ticket_id": w.ID("ticket.other", 4), "message": "staff reply"}, Label: "staff reply"},
				{Persona: Admin, Path: path, Body: map[string]any{"ticket_id": w.ID("ticket.user.closed", 0), "message": "closed"}, Label: "closed ticket"},
				{Persona: Admin, Path: path, Body: map[string]any{"ticket_id": Missing, "message": "m"}, Label: "unknown ticket"},
				{Persona: Admin, Path: path, Body: map[string]any{"ticket_id": w.ID("ticket.user.open", 3)}, Label: "missing message"},
				{Persona: User, Path: path, Body: map[string]any{"ticket_id": w.ID("ticket.user.open", 3), "message": "m"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "ticket.admin.ticket.id.close.post", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: fill("/api/v2/admin/ticket/:id/close", w.ID("ticket.other", 7)), Label: "close"},
				{Persona: Admin, Path: fill("/api/v2/admin/ticket/:id/close", w.ID("ticket.user.closed", 1)), Label: "close closed"},
				{Persona: Admin, Path: fill("/api/v2/admin/ticket/:id/close", Missing), Label: "unknown ticket"},
				{Persona: Admin, Path: "/api/v2/admin/ticket/x/close", Label: "invalid id"},
			}
		}},
	)
}

// seedKnowledge writes visible and hidden articles in Chinese, English and
// Japanese, with ties in sort order the handlers must break the same way.
func seedKnowledge(s *Seeder) {
	type lang struct{ category, title, body string }
	langs := []lang{
		{"帮助", "如何使用客户端 %d", "<p>第 %d 篇：下载客户端并导入订阅。</p>"},
		{"Help", "Getting started %d", "<p>Article %d: import the subscription link.</p>"},
		{"ヘルプ", "使い方 %d", "<p>記事 %d：サブスクリプションをインポートします。</p>"},
		{"公告", "维护通知 %d", "<p>维护 %d：节点 example.com 将于周末重启。</p>"},
	}
	total := 48 * s.Scale
	for i := 0; i < total; i++ {
		l := langs[i%len(langs)]
		show := 1
		if i%6 == 5 {
			show = 0
		}
		created := s.At(Days(-120+i) + time.Duration(s.Rand.Intn(3600))*time.Second)
		article := model.Knowledge{
			Category: l.category, Title: fmt.Sprintf(l.title, i+1), Body: fmt.Sprintf(l.body, i+1),
			Sort: s.Rand.Intn(5), Show: show, CreatedAt: created, UpdatedAt: created.Add(time.Duration(i) * time.Hour),
		}
		s.Create(&article)
		if show == 1 {
			s.World.Add("knowledge.visible", article.ID)
		} else {
			s.World.Add("knowledge.hidden", article.ID)
		}
	}
}

// seedTickets writes tickets of the user persona in every status, of user2
// and of other members, with member and administrator replies.
func seedTickets(s *Seeder) {
	userID, user2ID := s.World.PersonaID(User), s.World.PersonaID(User2)
	adminID := s.World.PersonaID(Admin)
	subjects := []string{"无法连接节点", "Payment not credited", "订阅链接失效", "速度很慢 slow", "退款申请 refund", "アカウントの問題"}
	create := func(owner uint, status int, key string, replies int, offset int) {
		created := s.At(Days(-60+offset) + time.Duration(s.Rand.Intn(7200))*time.Second)
		ticket := model.Ticket{
			UserID: owner, Subject: subjects[s.Rand.Intn(len(subjects))], Level: s.Rand.Intn(3), Status: status,
			CreatedAt: created, UpdatedAt: created.Add(time.Duration(replies+1) * time.Hour),
		}
		s.Create(&ticket)
		s.World.Add(key, ticket.ID)
		for r := 0; r <= replies; r++ {
			fromAdmin := r%2 == 1
			author, isAdmin := owner, 0
			message := fmt.Sprintf("member message %d on ticket %d", r, ticket.ID)
			if fromAdmin {
				author, isAdmin, message = adminID, 1, fmt.Sprintf("管理员回复 %d", r)
			}
			s.Create(&model.TicketMessage{TicketID: ticket.ID, UserID: author, Message: message, IsAdmin: isAdmin, CreatedAt: created.Add(time.Duration(r) * time.Hour)})
		}
	}
	for i := 0; i < 6; i++ {
		create(userID, 0, "ticket.user.open", i%3, i)
		create(userID, 1, "ticket.user.answered", 1+i%2, 10+i)
		create(userID, 2, "ticket.user.closed", 2, 20+i)
	}
	for i := 0; i < 4; i++ {
		create(user2ID, i%3, "ticket.user2", i, 30+i)
	}
	members := s.World.IDs["user.member"]
	for i := 0; i < 90*s.Scale; i++ {
		owner := members[2+s.Rand.Intn(len(members)-2)]
		create(owner, s.Rand.Intn(3), "ticket.other", s.Rand.Intn(4), s.Rand.Intn(60))
	}
}
