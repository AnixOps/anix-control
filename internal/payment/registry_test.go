package payment

import "testing"

type fakeGateway struct {
	typ string
}

func (f fakeGateway) Type() string { return f.typ }
func (f fakeGateway) VerifyCallback(ctx *CallbackContext) (*CallbackResult, error) {
	return &CallbackResult{TradeNo: "T1", Status: StatusPaid}, nil
}

func TestRegisterAndGet(t *testing.T) {
	// 用独立 type 避免与真实注册的网关冲突。
	Register(fakeGateway{typ: "test-fake-gw"})

	gw, ok := Get("test-fake-gw")
	if !ok {
		t.Fatal("expected gateway to be registered")
	}
	if gw.Type() != "test-fake-gw" {
		t.Fatalf("expected type test-fake-gw, got %s", gw.Type())
	}

	_, ok = Get("nonexistent-gw")
	if ok {
		t.Fatal("expected nonexistent gateway lookup to fail")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	Register(fakeGateway{typ: "dup-gw"})
	Register(fakeGateway{typ: "dup-gw"}) // 应 panic
}

func TestRegisterEmptyTypePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty type")
		}
	}()
	Register(fakeGateway{typ: ""})
}

func TestTypesSorted(t *testing.T) {
	Register(fakeGateway{typ: "zzz-gw"})
	Register(fakeGateway{typ: "aaa-gw"})

	types := Types()
	// 验证有序：前一个必须 <= 后一个。
	for i := 1; i < len(types); i++ {
		if types[i-1] > types[i] {
			t.Fatalf("Types() not sorted: %v", types)
		}
	}
}
