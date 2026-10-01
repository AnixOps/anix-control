package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// Forward runtime job payloads without tokens (node-ops-service.md
// sections 3.7 and 4.1, NO-7).
//
// Until NO-7 a v2_forward_runtime_job payload copied the ingress node's API
// token (panelForward.ingressNode.apiToken), so the job table, the
// administrator's job list and a clean agent's claim all carried it. Now:
//
//   - new payloads carry no token: the gost executor resolves it when it
//     sends the request to NodeX (ForwardNodeAPIToken), and the bridge
//     worker when it asks NodeX to translate a clean agent job;
//   - rows written before this change are scrubbed at start
//     (ScrubForwardRuntimeJobPayloads), and every reader scrubs what it
//     serves meanwhile (ScrubForwardRuntimeJobPayload), so both shapes are
//     served during the transition.

// ErrForwardNodeEndpointUnconfirmed is nodesecrets.ErrEndpointUnconfirmed
// as the forward runtime reports it: the kernel did not present the node's
// token because the node's address is not its pinned endpoint.
var ErrForwardNodeEndpointUnconfirmed = nodesecrets.ErrEndpointUnconfirmed

// ForwardNodeAPIToken is the token the kernel presents to a forward node's
// management API, through NodeX or gost, at the node's current address:
// its token when the address is the pinned endpoint (section 3.8), else
// ErrForwardNodeEndpointUnconfirmed.
func ForwardNodeAPIToken(db *gorm.DB, node *model.ForwardNode) (string, error) {
	token, err := nodesecrets.ForwardNodeTokenAt(db, node)
	if errors.Is(err, nodesecrets.ErrEndpointUnconfirmed) {
		return "", fmt.Errorf("forward node %d: %w", node.ID, ErrForwardNodeEndpointUnconfirmed)
	}
	return token, err
}

// ScrubForwardRuntimeJobPayload removes every credential from a stored job
// payload: every key IsNodeSecretKey marks, at any depth, is removed with
// its value, as a new payload omits it. It reports whether anything was
// removed. A payload that is not JSON is returned as it is. The key is
// removed, not masked: the placeholder would read as a token present to
// NodeX and the agents, and a cleared value would be scanned again at the
// next start.
func ScrubForwardRuntimeJobPayload(payload string) (string, bool) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return payload, false
	}
	value, ok := decodeNodeSecretJSON(trimmed)
	if !ok {
		return payload, false
	}
	scrubbed, changed := clearNodeSecretValues(value)
	if !changed {
		return payload, false
	}
	return encodeNodeSecretJSON(scrubbed), true
}

// clearNodeSecretValues removes the members under secret keys, whatever
// they hold, and walks the rest.
func clearNodeSecretValues(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for key, item := range typed {
			if IsNodeSecretKey(key) {
				delete(typed, key)
				changed = true
				continue
			}
			if scrubbed, itemChanged := clearNodeSecretValues(item); itemChanged {
				typed[key], changed = scrubbed, true
			}
		}
		return typed, changed
	case []any:
		changed := false
		for index, item := range typed {
			if scrubbed, itemChanged := clearNodeSecretValues(item); itemChanged {
				typed[index], changed = scrubbed, true
			}
		}
		return typed, changed
	}
	return value, false
}

// DefaultForwardRuntimeJobScrubBatch bounds the rows one scrub transaction
// rewrites.
const DefaultForwardRuntimeJobScrubBatch = 200

// ScrubForwardRuntimeJobPayloads rewrites the payloads of the runtime jobs
// written before NO-7 that still carry a token, in batches, and answers
// how many rows it changed. It is idempotent: a scrubbed payload no longer
// matches, so a start after the first pass reads nothing. It runs at start
// in the singleton worker process (cmd/server), before the job executors
// serve a row.
func ScrubForwardRuntimeJobPayloads(ctx context.Context, db *gorm.DB, batch int) (int64, error) {
	if db == nil {
		return 0, errors.New("database is required")
	}
	if batch <= 0 {
		batch = DefaultForwardRuntimeJobScrubBatch
	}
	var scrubbed int64
	lastID := uint(0)
	for {
		if err := ctx.Err(); err != nil {
			return scrubbed, err
		}
		var rows []model.ForwardRuntimeJob
		// Payloads that still name the token key the old executors wrote;
		// a scrubbed payload and a new one carry no key, so they are not
		// read again.
		if err := db.WithContext(ctx).Select("id", "payload").
			Where("id > ? AND payload LIKE ?", lastID, `%"apiToken":%`).
			Order("id").Limit(batch).Find(&rows).Error; err != nil {
			return scrubbed, err
		}
		if len(rows) == 0 {
			return scrubbed, nil
		}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				payload, changed := ScrubForwardRuntimeJobPayload(row.Payload)
				if !changed {
					continue
				}
				if err := tx.Model(&model.ForwardRuntimeJob{}).Where("id = ?", row.ID).Update("payload", payload).Error; err != nil {
					return err
				}
				scrubbed++
			}
			return nil
		})
		if err != nil {
			return scrubbed, err
		}
		lastID = rows[len(rows)-1].ID
	}
}

// RunForwardRuntimeJobPayloadScrub is the start-up pass: it scrubs and
// logs counts only.
func RunForwardRuntimeJobPayloadScrub(ctx context.Context, db *gorm.DB) {
	scrubbed, err := ScrubForwardRuntimeJobPayloads(ctx, db, DefaultForwardRuntimeJobScrubBatch)
	switch {
	case err != nil && !errors.Is(err, context.Canceled):
		log.Printf("Forward runtime jobs: scrubbing stored payloads failed after %d rows: %v", scrubbed, err)
	case scrubbed > 0:
		log.Printf("Forward runtime jobs: removed the node tokens from %d stored payloads", scrubbed)
	}
}

// scrubForwardRuntimeJobs scrubs the payloads of jobs about to be answered.
func scrubForwardRuntimeJobs(jobs []model.ForwardRuntimeJob) {
	for i := range jobs {
		jobs[i].Payload, _ = ScrubForwardRuntimeJobPayload(jobs[i].Payload)
	}
}
