package syncx

import "testing"

func TestShardedMapStoreLoadDelete(t *testing.T) {
	m := NewShardedMap[string, int](8, nil)

	m.Store("a", 1)

	value, ok := m.Load("a")
	if !ok {
		t.Fatalf("expected key a to exist")
	}
	if value != 1 {
		t.Fatalf("expected value 1, got %d", value)
	}

	m.Delete("a")
	if _, ok := m.Load("a"); ok {
		t.Fatalf("expected key a to be deleted")
	}
}

func TestShardedMapUpdate(t *testing.T) {
	m := NewShardedMap[string, int](4, nil)
	m.Store("a", 1)

	updated := m.Update("a", func(value int, ok bool) (int, bool) {
		if !ok {
			t.Fatalf("expected key a to exist during update")
		}
		return value + 1, true
	})
	if !updated {
		t.Fatalf("expected update to report success")
	}

	value, ok := m.Load("a")
	if !ok || value != 2 {
		t.Fatalf("expected updated value 2, got %d exists=%v", value, ok)
	}
}

func TestShardedMapRange(t *testing.T) {
	m := NewShardedMap[string, int](2, nil)
	m.Store("a", 1)
	m.Store("b", 2)

	total := 0
	m.Range(func(_ string, value int) bool {
		total += value
		return true
	})

	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
}
