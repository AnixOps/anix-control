package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSplitByClientGivesPickedClientsTheirOwnBudget(t *testing.T) {
	relay := net.ParseIP("203.0.113.7")
	public := NewRateLimiter(1, 2, WithTestModeBypass(false))
	relays := NewRateLimiter(1, 5, WithTestModeBypass(false))
	engine := gin.New()
	engine.Use(public.SplitByClient(func(ip net.IP) bool { return ip.Equal(relay) }, relays))
	engine.GET("/install.sh", func(c *gin.Context) { c.Status(http.StatusOK) })

	count := func(remote string, n int) (ok int) {
		for range n {
			req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
			req.RemoteAddr = remote
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				ok++
			} else if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("status %d", rec.Code)
			}
		}
		return ok
	}
	if got := count("203.0.113.7:4000", 8); got != 5 {
		t.Fatalf("relay got %d requests through, want its burst of 5", got)
	}
	if got := count("198.51.100.9:4000", 8); got != 2 {
		t.Fatalf("other client got %d requests through, want the public burst of 2", got)
	}
}

func TestSplitByClientWithoutPickUsesOwnLimit(t *testing.T) {
	public := NewRateLimiter(1, 1, WithTestModeBypass(false))
	engine := gin.New()
	engine.Use(public.SplitByClient(nil, nil))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	codes := []int{}
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "198.51.100.9:4000"
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v", codes)
	}
}
