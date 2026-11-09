package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/entrypool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EntryPoolsHandler is the administrator API of entry pools
// (internal/entrypool): node entries dealt out to subscribers by pool, a
// quarantine pool for new and trial accounts, and the lookup of who was
// given a leaked address. Every write is in the operation log (module
// entry_pool).
type EntryPoolsHandler struct {
	db func() *gorm.DB
}

// NewEntryPoolsHandler reads the database on each request.
func NewEntryPoolsHandler() *EntryPoolsHandler {
	return &EntryPoolsHandler{db: database.Get}
}

func (h *EntryPoolsHandler) service() *entrypool.Service { return entrypool.New(h.db()) }

func (h *EntryPoolsHandler) actor(c *gin.Context) entrypool.Actor {
	return entrypool.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

func (h *EntryPoolsHandler) poolError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, entrypool.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, entrypool.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// Overview godoc
// @Summary Entry pools
// @Description The policy (shards, quarantine_days, quarantine_pool) and every pool with entries or subscribers: its node entries, how many subscribers it has and why (assigned, quarantine, shard, none), and up to 500 of their user ids. The pool "" holds the shared entries and the subscribers without a pool.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entry-pools [get]
func (h *EntryPoolsHandler) Overview(c *gin.Context) {
	view, err := h.service().Overview(c.Request.Context())
	if err != nil {
		h.poolError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// SetPolicy godoc
// @Summary Set the entry pool policy
// @Description shards deals the subscribers without an assignment into shard-0..shard-(N-1) by user id (0 off, 2-64); quarantine_days puts accounts younger than that, and accounts without a plan, into quarantine_pool (0 off). Subscriptions follow on their next update. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body entrypool.Policy true "the policy"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entry-pools/policy [put]
func (h *EntryPoolsHandler) SetPolicy(c *gin.Context) {
	var input entrypool.Policy
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	policy, err := h.service().SetPolicy(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.poolError(c, err)
		return
	}
	kernelData(c, http.StatusOK, policy)
}

// Assign godoc
// @Summary Assign subscribers to an entry pool
// @Description Puts the users in pool (a canary pool of one user, or the pool a leak is isolated in), over the policy; an empty pool removes their assignment. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body entrypool.AssignInput true "users and pool"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entry-pools/assign [post]
func (h *EntryPoolsHandler) Assign(c *gin.Context) {
	var input entrypool.AssignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	n, err := h.service().Assign(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.poolError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"users": n, "pool": input.Pool})
}

// Split godoc
// @Summary Split an entry pool in two
// @Description Assigns the subscribers of pool alternately to into[0] and into[1], to bisect a leak: give each half its own entry and see which one leaks. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body entrypool.SplitInput true "the pool and its halves"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entry-pools/split [post]
func (h *EntryPoolsHandler) Split(c *gin.Context) {
	var input entrypool.SplitInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service().Split(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.poolError(c, err)
		return
	}
	kernelData(c, http.StatusOK, result)
}

// Leak godoc
// @Summary Who was given a leaked address
// @Description The entries with entry_id, or with host (an address found in a leaked subscription or probed), each with its pool and the subscribers of that pool (up to 500 ids). An entry without a pool went to every subscriber without an entry of their own pool on that node.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param entry_id query int false "node entry id"
// @Param host query string false "entry host"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entry-pools/leak [get]
func (h *EntryPoolsHandler) Leak(c *gin.Context) {
	input := entrypool.LeakInput{Host: c.Query("host")}
	if raw := c.Query("entry_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", "entry_id must be a number")
			return
		}
		input.EntryID = uint(id)
	}
	matches, err := h.service().Leak(c.Request.Context(), input)
	if err != nil {
		h.poolError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"matches": matches})
}
