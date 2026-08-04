package manager

import (
	"testing"

	APIModule "github.com/cpmores/lucinda/api/v1/module"
)

type stubModule struct {
	id     APIModule.ModuleID
	typ    APIModule.ModuleType
	health APIModule.ModuleHealth
}

func (s *stubModule) GetModuleID() APIModule.ModuleID               { return s.id }
func (s *stubModule) GetModuleType() APIModule.ModuleType             { return s.typ }
func (s *stubModule) CheckHealth() APIModule.ModuleHealth              { return s.health }
func (s *stubModule) RegisterWithManager(m ModuleManager) error        { return nil }

func TestRegisterAndGet(t *testing.T) {
	mgr := NewModuleManager()
	mod := &stubModule{id: "eventbus-default", typ: APIModule.EVENTBUS}
	if err := mgr.Register(mod); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := mgr.Get("eventbus-default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GetModuleID() != "eventbus-default" {
		t.Fatalf("expected eventbus-default, got %s", got.GetModuleID())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	mgr := NewModuleManager()
	mod := &stubModule{id: "dup", typ: APIModule.EVENTBUS}
	mgr.Register(mod)
	if err := mgr.Register(mod); err == nil {
		t.Fatal("expected error on duplicate Register, got nil")
	}
}

func TestUnregister(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{id: "mod", typ: APIModule.EVENTBUS})
	if err := mgr.Unregister("mod"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if mgr.Exists("mod") {
		t.Fatal("Exists should be false after Unregister")
	}
}

func TestUnregisterNotFound(t *testing.T) {
	mgr := NewModuleManager()
	if err := mgr.Unregister("nope"); err == nil {
		t.Fatal("expected error on Unregister of missing module, got nil")
	}
}

func TestGetByType(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{id: "eb-1", typ: APIModule.EVENTBUS})
	mgr.Register(&stubModule{id: "eb-2", typ: APIModule.EVENTBUS})
	mgr.Register(&stubModule{id: "tr-1", typ: APIModule.TRANSPORT})

	if len(mgr.GetByType(APIModule.EVENTBUS)) != 2 {
		t.Fatal("expected 2 eventbus modules")
	}
	if len(mgr.GetByType(APIModule.TRANSPORT)) != 1 {
		t.Fatal("expected 1 transport module")
	}
}

func TestList(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{id: "a", typ: APIModule.EVENTBUS})
	mgr.Register(&stubModule{id: "b", typ: APIModule.TRANSPORT})
	mgr.Register(&stubModule{id: "c", typ: APIModule.HARDWAREMONITOR})
	if len(mgr.List()) != 3 {
		t.Fatal("expected 3 modules")
	}
}

func TestExists(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{id: "exists", typ: APIModule.EVENTBUS})
	if !mgr.Exists("exists") {
		t.Fatal("Exists should return true")
	}
	if mgr.Exists("nope") {
		t.Fatal("Exists should return false")
	}
}

func TestGetNotFound(t *testing.T) {
	mgr := NewModuleManager()
	if _, err := mgr.Get("nope"); err == nil {
		t.Fatal("expected error on Get of missing module")
	}
}

func TestGrantAndRequire(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Grant(APIModule.TRANSPORT, APIModule.EVENTBUS)

	target := &stubModule{id: "eb-default", typ: APIModule.EVENTBUS}
	mgr.Register(target)

	caller := &stubModule{id: "tr-default", typ: APIModule.TRANSPORT}
	got, err := mgr.Require(caller, APIModule.EVENTBUS, "eb-default")
	if err != nil {
		t.Fatalf("Require should succeed when granted: %v", err)
	}
	if got.GetModuleID() != "eb-default" {
		t.Fatalf("expected eb-default, got %s", got.GetModuleID())
	}
}

func TestRequireAccessDenied(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{id: "eb-default", typ: APIModule.EVENTBUS})

	caller := &stubModule{id: "tr-default", typ: APIModule.TRANSPORT}
	if _, err := mgr.Require(caller, APIModule.EVENTBUS, "eb-default"); err == nil {
		t.Fatal("expected access denied error")
	}
}

func TestGrantIdempotent(t *testing.T) {
	mgr := NewModuleManager()
	if err := mgr.Grant(APIModule.EVENTBUS, APIModule.TRANSPORT); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := mgr.Grant(APIModule.EVENTBUS, APIModule.TRANSPORT); err != nil {
		t.Fatalf("second Grant should be idempotent: %v", err)
	}
}

func TestHealth(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{
		id:  "mod-1",
		typ: APIModule.EVENTBUS,
		health: APIModule.ModuleHealth{Status: APIModule.Running},
	})
	h, err := mgr.Health("mod-1")
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != APIModule.Running {
		t.Fatalf("expected RUNNING, got %s", h.Status)
	}
}

func TestHealthNotFound(t *testing.T) {
	mgr := NewModuleManager()
	if _, err := mgr.Health("nope"); err == nil {
		t.Fatal("expected error on Health of missing module")
	}
}

func TestHealthAll(t *testing.T) {
	mgr := NewModuleManager()
	mgr.Register(&stubModule{id: "a", typ: APIModule.EVENTBUS, health: APIModule.ModuleHealth{Status: APIModule.Running}})
	mgr.Register(&stubModule{id: "b", typ: APIModule.TRANSPORT, health: APIModule.ModuleHealth{Status: APIModule.Stopped}})
	all := mgr.HealthAll()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestNewModuleManagerReturnsPointer(t *testing.T) {
	mgr1 := NewModuleManager()
	mgr2 := mgr1
	mgr1.Register(&stubModule{id: "shared", typ: APIModule.EVENTBUS})
	if !mgr2.Exists("shared") {
		t.Fatal("mgr2 should see module registered on mgr1")
	}
}
