package kernelnodeops

import (
	"context"
	"fmt"
	"sync"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/gost"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// backendTestTokenField is the request position the administrator's token
// is sealed from on POST /admin/forward/test-connection
// (config/node-secret-fields.json).
const backendTestTokenField = "/api_token"

// ConnectionTest executes diagnose.forward_backend (node-ops-service.md
// section 6.1, NO-7): the gost API connection test of
// POST /admin/forward/test-connection, run from the kernel with the token
// the administrator typed. The gost-mesh package host submits the
// operation with its request binding and the sealed handle of the token;
// the Preparer resolves the handle in the submitting call and keeps the
// value in kernel memory until the executor dials. The token is scrubbed
// from the result and never recorded.
//
// The dial follows the kernel's legacy handler: the same gost client, the
// same calls, the same error texts, so the route's answer is byte for byte
// what it was. A refusing or unreachable API is the result, not a failed
// operation. The SSRF rule of the diagnoses (#84, #90) applies: a request
// that is not an administrator's dials public addresses only.
type ConnectionTest struct {
	// Probes resolves the host for the public-address check; the zero
	// value uses the system resolver.
	Probes service.DiagnosisProbes
	// Now purges the held tokens; time.Now when nil.
	Now func() time.Time

	mu     sync.Mutex
	tokens map[string]heldToken
}

type heldToken struct {
	value string
	until time.Time
}

func (c *ConnectionTest) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Register serves the kind on registry.
func (c *ConnectionTest) Register(registry *Registry) error {
	return registry.Register(KindDiagnoseForwardBackend, c)
}

func init() {
	if err := (&ConnectionTest{}).Register(DefaultExecutors); err != nil {
		panic(err)
	}
}

// Prepare resolves the token's handle for the bound request (target kind
// dial, field /api_token) and holds the value for the executor until the
// request's deadline. Without a binding the handle does not resolve
// (PERMISSION_DENIED) and nothing is recorded.
func (c *ConnectionTest) Prepare(_ context.Context, submission *Submission) error {
	if submission.Request == nil {
		return status.Error(codes.PermissionDenied, "a connection test needs the request binding of the administrator's request")
	}
	op := submission.Operation.GetTestForwardBackend()
	token := ""
	if op.GetToken() != nil {
		secrets, err := submission.Unseal(sealedsecrets.Use{Handle: op.GetToken().GetHandle(), Target: DialTarget(), Field: backendTestTokenField})
		if err != nil {
			return err
		}
		token = secrets[0].Value
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = map[string]heldToken{}
	}
	c.purgeLocked()
	c.tokens[bindingKey(submission.Host.PackageID, submission.Request.RequestID)] = heldToken{value: token, until: submission.Request.Deadline}
	return nil
}

func (c *ConnectionTest) purgeLocked() {
	now := c.now()
	for key, held := range c.tokens {
		if !now.Before(held.until) {
			delete(c.tokens, key)
		}
	}
}

// take answers, once, the token held for a bound request.
func (c *ConnectionTest) take(packageID, requestID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeLocked()
	key := bindingKey(packageID, requestID)
	held, ok := c.tokens[key]
	delete(c.tokens, key)
	return held.value, ok
}

// Held reports how many tokens are held, for tests.
func (c *ConnectionTest) Held() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeLocked()
	return len(c.tokens)
}

func backendTestResult(success bool, message string, count int) *kernelnodeopsv1.OperationResult {
	return &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_ForwardBackendTest{ForwardBackendTest: &kernelnodeopsv1.ForwardBackendTestResult{
		Success: success, Message: message, ServiceCount: port(count),
	}}}
}

// Execute dials the gost API as the legacy handler does.
func (c *ConnectionTest) Execute(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetTestForwardBackend()
	if run.Request == nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID, "the request that sealed the token has ended", false)
	}
	token, ok := c.take(run.PackageID, run.Request.RequestID)
	if !ok {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID, "the token of the bound request is no longer held", false)
	}
	run.UseSecret(token)
	if !run.Request.Admin {
		if _, refused := c.Probes.PublicHost(ctx, op.GetHost()); refused != "" {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, refused, false)
		}
	}
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL); !ok {
		return outcome
	}
	client := gost.NewClient(&gost.Config{Host: fmt.Sprintf("http://%s:%d", op.GetHost(), op.GetApiPort()), APIToken: token})
	if err := client.HealthCheck(ctx); err != nil {
		if ctx.Err() != nil {
			return Cancelled("the connection test stopped before the API answered")
		}
		return Succeeded(backendTestResult(false, err.Error(), 0))
	}
	services, err := client.GetServices(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return Cancelled("the connection test stopped before the API answered")
		}
		return Succeeded(backendTestResult(false, "API connected but failed to get services: "+err.Error(), 0))
	}
	return Succeeded(backendTestResult(true, "Connection successful", services.Count))
}
