package hashmap

import (
	"fmt"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"jsouthworth.net/go/dyn"
	"jsouthworth.net/go/seq"
)

func assert(t *testing.T, b bool, msg string) {
	if !b {
		t.Fatal(msg)
	}
}

func BenchmarkPMapAssoc(b *testing.B) {
	b.ReportAllocs()
	m := Empty()
	for i := 0; i < b.N; i++ {
		m = m.Assoc(i, i)
	}
}

func BenchmarkTMapAssoc(b *testing.B) {
	b.ReportAllocs()
	m := Empty().AsTransient()
	for i := 0; i < b.N; i++ {
		m.Assoc(i, i)
	}
	m.AsPersistent()
}

func BenchmarkNativeMapAssoc(b *testing.B) {
	b.ReportAllocs()
	m := make(map[int]int)
	for i := 0; i < b.N; i++ {
		m[i] = i
	}
}

func BenchmarkNativeMapInterfaceAssoc(b *testing.B) {
	b.ReportAllocs()
	m := make(map[interface{}]interface{})
	for i := 0; i < b.N; i++ {
		m[i] = i
	}
}

func TestEmptyGeneratesSeededEmpty(t *testing.T) {
	assert(t, Empty().hashSeed != Empty().hashSeed,
		"Empty generated the same seed")
}

func TestNew(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("New requires even number of elements", prop.ForAll(
		func(elems []interface{}) (ok bool) {
			ok = true
			defer func() {
				_ = recover()
			}()
			_ = New(elems...)
			return false
		},
		gen.SliceOf(gen.Identifier(), reflect.TypeOf((*interface{})(nil)).Elem()).
			SuchThat(func(sl []interface{}) bool { return len(sl)%2 != 0 }),
	))
	properties.Property("New produces expected map", prop.ForAll(
		func(elems []interface{}) bool {
			m := New(elems...)
			exp := make(map[interface{}]interface{})
			for i := 0; i < len(elems); i = i + 2 {
				key := elems[i]
				val := elems[i+1]
				exp[key] = val
			}
			for key, val := range exp {
				if m.At(key) != val {
					fmt.Println(key, m.At(key), val)
					return false
				}
			}
			return true
		},
		gen.SliceOf(gen.Identifier(), reflect.TypeOf((*interface{})(nil)).Elem()).
			SuchThat(func(sl []interface{}) bool { return len(sl)%2 == 0 }),
	))
	properties.TestingRun(t)
}

func TestFrom(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("From(m) == m", prop.ForAll(
		func(rm *rmap) bool {
			new := From(rm.m)
			return new == rm.m
		},
		genRandomMap,
	))
	properties.Property("From(transient) stops mutation", prop.ForAll(
		func(rm *rmap, k, v string) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errTafterP
			}()
			t := rm.m.AsTransient()
			_ = From(t)
			t.Assoc(k, v)
			return false
		},
		genRandomMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("From(map[interface{}]interface{}) builds correct map", prop.ForAll(
		func(m map[interface{}]interface{}) bool {
			pm := From(m)
			for k, v := range m {
				if pm.At(k) != v {
					return false
				}
			}
			return true
		},
		gopter.DeriveGen(
			func(entries map[string]string) map[interface{}]interface{} {
				out := make(map[interface{}]interface{})
				for k, v := range entries {
					out[k] = v
				}
				return out

			},
			func(m map[interface{}]interface{}) map[string]string {
				out := make(map[string]string)
				for k, v := range m {
					out[k.(string)] = v.(string)
				}
				return out
			},
			gen.MapOf(gen.Identifier(), gen.Identifier()),
		),
	))
	properties.Property("From([]Entry) builds correct map", prop.ForAll(
		func(entries []Entry) bool {
			m := From(entries)
			for _, entry := range entries {
				if m.At(entry.Key()) != entry.Value() {
					return false
				}
			}
			return true
		},
		gen.SliceOf(gopter.DeriveGen(
			func(k, v string) Entry {
				return Entry(entry{k: k, v: v})
			},
			func(e Entry) (k, v string) {
				return e.Key().(string), e.Value().(string)
			},
			gen.Identifier(),
			gen.Identifier(),
		), reflect.TypeOf((*Entry)(nil)).Elem()).
			SuchThat(func(entries []Entry) bool {
				seen := make(map[string]struct{})
				for _, entry := range entries {
					_, ok := seen[entry.Key().(string)]
					if ok {
						return false
					}
					seen[entry.Key().(string)] = struct{}{}
				}
				return true
			}),
	))
	properties.Property("From([]interface{}) builds correct map", prop.ForAll(
		func(elems []interface{}) bool {
			m := From(elems)
			for i := 0; i < len(elems); i += 2 {
				k := elems[i]
				v := elems[i+1]
				if m.At(k) != v {
					return false
				}
			}
			return true
		},
		gen.SliceOf(gen.Identifier(),
			reflect.TypeOf((*interface{})(nil)).Elem()).
			SuchThat(func(sl []interface{}) bool {
				return len(sl)%2 == 0
			}).
			SuchThat(func(elems []interface{}) bool {
				seen := make(map[string]struct{})
				for i := 0; i < len(elems); i += 2 {
					k := elems[i]
					_, ok := seen[k.(string)]
					if ok {
						return false
					}
					seen[k.(string)] = struct{}{}
				}
				return true
			}),
	))
	properties.Property("From(map[T]T) builds correct map", prop.ForAll(
		func(in map[string]string) bool {
			m := From(in)
			for k, v := range in {
				if m.At(k) != v {
					return false
				}
			}
			return true
		},
		gen.MapOf(gen.Identifier(), gen.Identifier()),
	))
	properties.Property("From(map[kT]vT) builds correct map", prop.ForAll(
		func(in map[string]int) bool {
			m := From(in)
			for k, v := range in {
				if m.At(k) != v {
					return false
				}
			}
			return true
		},
		gen.MapOf(gen.Identifier(), gen.Int()),
	))
	properties.Property("From([]T) builds correct map", prop.ForAll(
		func(elems []string) bool {
			m := From(elems)
			for i := 0; i < len(elems); i += 2 {
				k := elems[i]
				v := elems[i+1]
				if m.At(k) != v {
					return false
				}
			}
			return true
		},
		gen.SliceOf(gen.Identifier()).
			SuchThat(func(sl []string) bool {
				return len(sl)%2 == 0
			}).
			SuchThat(func(elems []string) bool {
				seen := make(map[string]struct{})
				for i := 0; i < len(elems); i += 2 {
					k := elems[i]
					_, ok := seen[k]
					if ok {
						return false
					}
					seen[k] = struct{}{}
				}
				return true
			}),
	))
	properties.Property("From(int) returns empty", prop.ForAll(
		func(i int) bool {
			m := From(i)
			return m.Length() == 0
		},
		gen.Int(),
	))
	properties.TestingRun(t)
}

func TestAt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.At(entry.k)==entry.v", prop.ForAll(
		func(rm *rmap) bool {
			for key, val := range rm.entries {
				if val != rm.m.At(key) {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.TestingRun(t)
}

func TestApply(t *testing.T) {
	m := New("a", 1, "b", 2)
	if dyn.Apply(m, "a") != m.At("a") {
		t.Fatal("Apply didn't return the expected value")
	}
}

func TestEntryAt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.EntryAt(entry.k).Value()==entry.v", prop.ForAll(
		func(rm *rmap) bool {
			for key, val := range rm.entries {
				entry := rm.m.EntryAt(key)
				if entry.Key() != key || entry.Value() != val {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.Property("new=large.Delete(k) -> new.EntryAt(k)==nil && large.EntryAt(k)==entry{k,v}", prop.ForAll(
		func(lm *lmap) bool {
			key := lm.k + strconv.Itoa(lm.num-1)
			val := lm.v + strconv.Itoa(lm.num-1)
			new := lm.m.Delete(key)
			return new.EntryAt(key) == nil &&
				lm.m.EntryAt(key).Key() == key &&
				lm.m.EntryAt(key).Value() == val
		},
		genLargeMap,
	))
	properties.TestingRun(t)
}

func TestContains(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.Contains(entry.k)", prop.ForAll(
		func(rm *rmap) bool {
			for key := range rm.entries {
				if !rm.m.Contains(key) {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.TestingRun(t)
}

func TestFind(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.Find(entry.k) is non-nil and exists", prop.ForAll(
		func(rm *rmap) bool {
			for key := range rm.entries {
				v, ok := rm.m.Find(key)
				if v == nil || !ok {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.Property("Non-existent keys don't exist in map", prop.ForAll(
		func(rm *rmap, key string) bool {
			_, inEntries := rm.entries[key]
			_, inMap := rm.m.Find(key)
			return inEntries == inMap
		},
		genRandomMap,
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestAssoc(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new = empty.Assoc(k,v) -> new != empty ", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Assoc(k, v)
			return new != m
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=empty.Assoc(k, v) -> new.At(k)==v", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Assoc(k, v)
			got := new.At(k)
			return got == v
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=empty.Assoc(k, v) -> empty.At(k)!=v", prop.ForAll(
		func(m *Map, k, v string) bool {
			m.Assoc(k, v)
			got := m.At(k)
			return got != v
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("one=empty.Assoc(k, v); two=one.Assoc(k, v) -> one==two", prop.ForAll(
		func(m *Map, k, v string) bool {
			one := m.Assoc(k, v)
			two := one.Assoc(k, v)
			return one == two
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("one=empty.Assoc(k, v1); two=one.Assoc(k, v2) -> one != two", prop.ForAll(
		func(m *Map, k, v1, v2 string) bool {
			one := m.Assoc(k, v1)
			two := one.Assoc(k, v2)
			return v1 == v2 || one != two
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.Property("one=empty.Assoc(k, v1); two=one.Assoc(k, v2) -> one.At(k)!=two.At(k)", prop.ForAll(
		func(m *Map, k, v1, v2 string) bool {
			one := m.Assoc(k, v1)
			two := one.Assoc(k, v2)
			return v1 == v2 || one.At(k) != two.At(k)
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.Property("new=large.Assoc(k,v) -> new!=empty ", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.Assoc(k, v)
			return new != lm.m
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k, v) -> new.At(k)==v", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.Assoc(k, v)
			got := new.At(k)
			return got == v
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k, v) -> empty.At(k)!=v", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			lm.m.Assoc(k, v)
			got := lm.m.At(k)
			return got != v
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("one=large.Assoc(k, v); two=one.Assoc(k, v) -> one==two", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			one := lm.m.Assoc(k, v)
			two := one.Assoc(k, v)
			return one == two
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("one=large.Assoc(k, v1); two=one.Assoc(k, v2) -> one!=two", prop.ForAll(
		func(lm *lmap, k, v1, v2 string) bool {
			one := lm.m.Assoc(k, v1)
			two := one.Assoc(k, v2)
			return v1 == v2 || one != two
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("one=large.Assoc(k, v1); two=one.Assoc(k, v2) -> one.At(k)!=two.At(k)", prop.ForAll(
		func(lm *lmap, k, v1, v2 string) bool {
			one := lm.m.Assoc(k, v1)
			two := one.Assoc(k, v2)
			return v1 == v2 || one.At(k) != two.At(k)
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("ForAll k=0-lm.num, large.At(k) == v", prop.ForAll(
		func(lm *lmap) bool {
			for i := 0; i < lm.num; i++ {
				k := lm.k + strconv.Itoa(i)
				v := lm.v + strconv.Itoa(i)
				got := lm.m.At(k)
				if got != v {
					return false
				}
			}
			return true
		},
		genLargeMap,
	))

	properties.Property("one=random.Assoc(k, v1); two=one.Assoc(k, v2) -> one.At(k)!=two.At(k)", prop.ForAll(
		func(rm *rmap, k, v1, v2 string) bool {
			one := rm.m.Assoc(k, v1)
			two := one.Assoc(k, v2)
			return v1 == v2 || one.At(k) != two.At(k)
		},
		genRandomMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestConj(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new = empty.Assoc(k,v) -> new != empty ", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Conj(EntryNew(k, v))
			return new != m
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=empty.Assoc(k, v) -> new.At(k)==v", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Conj(EntryNew(k, v))
			got := new.(*Map).At(k)
			return got == v
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestAsTransient(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("empty.AsTransient().edit != zero", prop.ForAll(
		func(empty *Map, k, v string) bool {
			t := empty.AsTransient()
			return t.edit != zero
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestDelete(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new=empty.Delete(k) -> new==empty", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Delete(k)
			return new == m
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Delete(k) -> new!=large", prop.ForAll(
		func(lm *lmap) bool {
			new := lm.m.Delete(lm.k + strconv.Itoa(lm.num-1))
			return new != lm.m
		},
		genLargeMap,
	))
	properties.Property("new=large.Delete(k) -> new.At(k)==nil && large.At(k)==v", prop.ForAll(
		func(lm *lmap) bool {
			key := lm.k + strconv.Itoa(lm.num-1)
			val := lm.v + strconv.Itoa(lm.num-1)
			new := lm.m.Delete(key)
			return new.At(key) == nil && lm.m.At(key) == val
		},
		genLargeMap,
	))
	properties.Property("new=removeAll(large) -> new.Length()==0", prop.ForAll(
		func(lm *lmap) bool {
			new := lm.m
			for i := 0; i < lm.num; i++ {
				new = new.Delete(lm.k + strconv.Itoa(i))
			}
			return new.Length() == 0
		},
		genLargeMap,
	))
	properties.TestingRun(t)
}

func TestLength(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new=empty.Assoc(k, v) -> new.Length()==empty.Length()+1", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Assoc(k, v)
			return new.Length() == m.Length()+1
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k,v) -> new.Length()==large.Length()+1", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.Assoc(k, v)
			return lm.m.At(k) == v ||
				new.Length() == lm.m.Length()+1
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("lm.num == lm.m.Length()", prop.ForAll(
		func(lm *lmap) bool {
			return lm.m.Length() == lm.num
		},
		genLargeMap,
	))
	properties.Property("new=empty.Assoc(k, v).Delete(k) -> new.Length()==empty.Length()", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.Assoc(k, v).Delete(k)
			return new.Length() == m.Length()
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k,v).Delete(k) -> new.Length()==large.Length()", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.Assoc(k, v).Delete(k)
			return new.Length() == lm.m.Length()
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("lm.num == lm.m.Length()", prop.ForAll(
		func(lm *lmap) bool {
			return lm.m.Length() == lm.num
		},
		genLargeMap,
	))
	properties.Property("random.Length() increases correctly", prop.ForAll(
		func(rm *rmap, entries map[string]string) bool {
			m := rm.m
			count := m.Length()
			for key, val := range entries {
				if !m.Contains(key) {
					count++
				}
				m = m.Assoc(key, val)
			}
			return m.Length() == count
		},
		genRandomMap,
		gen.MapOf(gen.Identifier(), gen.Identifier()),
	))
	properties.TestingRun(t)
}

func TestAsNative(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("AsNative returns the full map", prop.ForAll(
		func(rm *rmap) bool {
			out := rm.m.AsNative()
			for k, v := range out {
				if v == rm.m.At(k) {
					continue
				}
				return false
			}
			return true
		},
		genRandomMap,
	))
	properties.TestingRun(t)
}

func TestEqual(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("m == m", prop.ForAll(
		func(rm *rmap) bool {
			return rm.m.Equal(rm.m)
		},
		genRandomMap,
	))
	properties.Property("new=m.Delete(k) -> new != m", prop.ForAll(
		func(rm *rmap) bool {
			var k string
			rm.m.Range(func(key, val string) bool {
				k = key
				return false
			})
			new := rm.m.Delete(k)
			return !rm.m.Equal(new)
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() != 0
		}),
	))
	properties.Property("m.Equal(10)==false", prop.ForAll(
		func(rm *rmap) bool {
			return !rm.m.Equal(10)
		},
		genRandomMap,
	))
	properties.Property("new=m.Assoc(k,v) -> new != m", prop.ForAll(
		func(rm *rmap) bool {
			var k string
			rm.m.Range(func(key, val string) bool {
				k = key
				return false
			})
			new := rm.m.Assoc(k, "foo")
			return !rm.m.Equal(new)
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() != 0
		}),
	))
	properties.TestingRun(t)
}

func TestRange(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("Range access the full map", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.Range(func(key, val interface{}) bool {
				if !foundAll {
					return false
				}
				foundAll = rm.entries[key.(string)] == val
				return true
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range access the full map no continue", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.Range(func(key, val interface{}) {
				if !foundAll {
					return
				}
				foundAll = rm.entries[key.(string)] == val
				return
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range access the full map entries", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.Range(func(entry Entry) bool {
				if !foundAll {
					return false
				}
				foundAll = rm.entries[entry.Key().(string)] == entry.Value()
				return true
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range access the full map entries no continue", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.Range(func(entry Entry) {
				if !foundAll {
					return
				}
				foundAll = rm.entries[entry.Key().(string)] == entry.Value()
				return
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range with reflected func", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.Range(func(key, val string) bool {
				if !foundAll {
					return false
				}
				foundAll = rm.entries[key] == val
				return true
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range with reflected func no continue", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.Range(func(key, val string) {
				if !foundAll {
					return
				}
				foundAll = rm.entries[key] == val
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range panics when passed a non function", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.Range(1)
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong number of inputs", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.Range(func(a, b, c string) {})
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong number of outputs", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.Range(func(a, b, c string) (d, e bool) { return false, false })
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong output type", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.Range(func(a, b string) string { return "" })
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong input types", prop.ForAll(
		func(rm *rmap) (ok bool) {
			ok = true
			defer func() {
				_ = recover()
			}()

			rm.m.Range(func(a, b int) {})
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.TestingRun(t)
	t.Run("Range works with nilable type", func(t *testing.T) {
		defer func() {
			r := recover()
			if r != nil {
				t.Fatal(r)
			}
		}()
		m := Empty().Assoc("a", nil)
		m.Range(func(k string, v *int) {
		})
	})
}

func TestSeq(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("Mutate makes expected changes", prop.ForAll(
		func(rm *rmap) (ok bool) {
			s := seq.Seq(rm.m)
			if s == nil {
				return true
			}
			foundAll := true
			fn := func(entry Entry) bool {
				foundAll = rm.entries[entry.Key().(string)] == entry.Value()
				return true
			}
			var cont = true
			for s != nil && cont {
				entry := seq.First(s).(Entry)
				cont = fn(entry)
				s = seq.Seq(seq.Next(s))
			}
			return foundAll
		},
		genRandomMap,
	))
	properties.TestingRun(t)
}

func TestTransform(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("Mutate makes expected changes", prop.ForAll(
		func(m *Map) bool {
			new := m.Transform(
				func(t *TMap) *TMap {
					return t.Assoc("foo", "bar")
				},
				func(t *TMap) *TMap {
					return t.Assoc("bar", "baz")
				},
			)
			return new.At("foo") == "bar" &&
				new.At("bar") == "baz"
		},
		genMap,
	))
	properties.TestingRun(t)
}

func TestString(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("Mutate makes expected changes", prop.ForAll(
		func(m *Map) bool {
			new := m.Transform(
				func(t *TMap) *TMap {
					return t.Assoc("foo", "bar")
				},
				func(t *TMap) *TMap {
					return t.Assoc("bar", "baz")
				},
			)
			return new.String() == "{ [foo bar] [bar baz] }" ||
				new.String() == "{ [bar baz] [foo bar] }"
		},
		genMap,
	))
	properties.TestingRun(t)
}

func TestTransientString(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("Mutate makes expected changes", prop.ForAll(
		func(m *Map) bool {
			new := m.Transform(
				func(t *TMap) *TMap {
					return t.Assoc("foo", "bar")
				},
				func(t *TMap) *TMap {
					return t.Assoc("bar", "baz")
				},
			).AsTransient()
			return new.String() == "{ [foo bar] [bar baz] }" ||
				new.String() == "{ [bar baz] [foo bar] }"
		},
		genMap,
	))
	properties.TestingRun(t)
}

func TestTransientAt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.At(entry.k)==entry.v", prop.ForAll(
		func(rm *rmap) bool {
			t := rm.m.AsTransient()
			for key, val := range rm.entries {
				if val != t.At(key) {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.TestingRun(t)
}

func TestTransientEntryAt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.EntryAt(entry.k).Value()==entry.v", prop.ForAll(
		func(rm *rmap) bool {
			for key, val := range rm.entries {
				entry := rm.m.AsTransient().EntryAt(key)
				if entry.Key() != key || entry.Value() != val {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.Property("new=large.Delete(k) -> new.EntryAt(k)==nil && large.EntryAt(k)==nil", prop.ForAll(
		func(lm *lmap) bool {
			key := lm.k + strconv.Itoa(lm.num-1)
			large := lm.m.AsTransient()
			new := large.Delete(key)
			return new.EntryAt(key) == nil &&
				large.EntryAt(key) == nil
		},
		genLargeMap,
	))
	properties.TestingRun(t)
}

func TestTransientContains(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.Contains(entry.k)", prop.ForAll(
		func(rm *rmap) bool {
			t := rm.m.AsTransient()
			for key := range rm.entries {
				if !t.Contains(key) {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.TestingRun(t)
}

func TestTransientFind(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("ForAll generatedEntries random.Find(entry.k) is non-nil and exists", prop.ForAll(
		func(rm *rmap) bool {
			t := rm.m.AsTransient()
			for key := range rm.entries {
				v, ok := t.Find(key)
				if v == nil || !ok {
					return false
				}
			}
			return true
		},
		genRandomMap,
	))
	properties.Property("Non-existent keys don't exist in map", prop.ForAll(
		func(rm *rmap, key string) bool {
			t := rm.m.AsTransient()
			_, inEntries := rm.entries[key]
			_, inMap := t.Find(key)
			return inEntries == inMap
		},
		genRandomMap,
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestTransientAssoc(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new=empty.AsTransient().Assoc(k,v) -> new != empty ", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.AsTransient().Assoc(k, v).AsPersistent()
			return new != m
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=empty.Assoc(k, v) -> new.At(k)==v", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.AsTransient().Assoc(k, v)
			got := new.At(k)
			return got == v
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=empty.Assoc(k, v) -> empty.At(k)!=v", prop.ForAll(
		func(m *Map, k, v string) bool {
			m.AsTransient().Assoc(k, v)
			got := m.At(k)
			return got != v
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.Property("one=empty.Assoc(k, v1); two=one.Assoc(k, v2) -> one.At(k)==two.At(k)", prop.ForAll(
		func(m *Map, k, v1, v2 string) bool {
			one := m.AsTransient().Assoc(k, v1)
			two := one.Assoc(k, v2)
			return one.At(k) == two.At(k)
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.Property("new=large.Assoc(k, v) -> new.At(k)==v", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.AsTransient().Assoc(k, v)
			got := new.At(k)
			return got == v
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k, v) -> empty.At(k)!=v", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			lm.m.AsTransient().Assoc(k, v)
			got := lm.m.At(k)
			return got != v
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("one=large.Assoc(k, v1); two=one.Assoc(k, v2) -> one.At(k)==two.At(k)", prop.ForAll(
		func(lm *lmap, k, v1, v2 string) bool {
			one := lm.m.AsTransient().Assoc(k, v1)
			two := one.Assoc(k, v2)
			return one.At(k) == two.At(k)
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("ForAll k=0-lm.num, large.At(k) == v", prop.ForAll(
		func(lm *lmap) bool {
			t := lm.m.AsTransient()
			for i := 0; i < lm.num; i++ {
				k := lm.k + strconv.Itoa(i)
				v := lm.v + strconv.Itoa(i)
				got := t.At(k)
				if got != v {
					return false
				}
			}
			return true
		},
		genLargeMap,
	))
	properties.TestingRun(t)
}

func TestTransientConj(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new=empty.Assoc(k, v) -> new.At(k)==v", prop.ForAll(
		func(m *Map, k, v string) bool {
			t := m.AsTransient()
			new := t.Conj(EntryNew(k, v))
			got := new.(*TMap).At(k)
			return got == v
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestTransientDelete(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new=large.Delete(k) -> new!=large", prop.ForAll(
		func(lm *lmap) bool {
			new := lm.m.AsTransient().Delete(lm.k + strconv.Itoa(lm.num-1)).AsPersistent()
			return new != lm.m
		},
		genLargeMap,
	))
	properties.Property("new=large.Delete(k) -> new.At(k)==nil && large.At(k)==v", prop.ForAll(
		func(lm *lmap) bool {
			key := lm.k + strconv.Itoa(lm.num-1)
			val := lm.v + strconv.Itoa(lm.num-1)
			new := lm.m.AsTransient().Delete(key)
			return new.At(key) == nil && lm.m.At(key) == val
		},
		genLargeMap,
	))
	properties.Property("new=removeAll(large) -> new.Length()==0", prop.ForAll(
		func(lm *lmap) bool {
			new := lm.m.AsTransient()
			for i := 0; i < lm.num; i++ {
				new = new.Delete(lm.k + strconv.Itoa(i))
			}
			return new.Length() == 0
		},
		genLargeMap,
	))
	properties.TestingRun(t)
}

func TestTransientLength(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("new=empty.Assoc(k, v) -> new.Length()==empty.Length()+1", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.AsTransient().Assoc(k, v)
			return new.Length() == m.Length()+1
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k,v) -> new.Length()==large.Length()+1", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.AsTransient().Assoc(k, v)
			return new.Length() == lm.m.Length()+1
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("lm.num == lm.m.Length()", prop.ForAll(
		func(lm *lmap) bool {
			return lm.m.AsTransient().Length() == lm.num
		},
		genLargeMap,
	))
	properties.Property("new=empty.Assoc(k, v).Delete(k) -> new.Length()==empty.Length()", prop.ForAll(
		func(m *Map, k, v string) bool {
			new := m.AsTransient().Assoc(k, v).Delete(k)
			return new.Length() == m.Length()
		},
		genMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("new=large.Assoc(k,v).Delete(k) -> new.Length()==large.Length()", prop.ForAll(
		func(lm *lmap, k, v string) bool {
			new := lm.m.AsTransient().Assoc(k, v).Delete(k)
			return new.Length() == lm.m.Length()
		},
		genLargeMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.Property("random.Length() increases correctly", prop.ForAll(
		func(rm *rmap, entries map[string]string) bool {
			m := rm.m.AsTransient()
			count := m.Length()
			for key, val := range entries {
				if !m.Contains(key) {
					count++
				}
				m = m.Assoc(key, val)
			}
			return m.Length() == count
		},
		genRandomMap,
		gen.MapOf(gen.Identifier(), gen.Identifier()),
	))
	properties.TestingRun(t)
}

func TestTransientAsPersistent(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("t.AsPersistent() -> atomic.LoadUint32(t.edit)==0", prop.ForAll(
		func(rm *rmap) bool {
			t := rm.m.AsTransient()
			t.AsPersistent()
			return atomic.LoadUint32(t.edit) == 0
		},
		genRandomMap,
	))
	properties.Property("t.AsPersistent() -> t.Assoc(k, v) panics with TafterP", prop.ForAll(
		func(rm *rmap, k, v string) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errTafterP
			}()
			t := rm.m.AsTransient()
			t.AsPersistent()
			t.Assoc(k, v)
			return false
		},
		genRandomMap,
		gen.Identifier(),
		gen.Identifier(),
	))
	properties.TestingRun(t)
}

func TestTransientEqual(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("m == m", prop.ForAll(
		func(rm *rmap) bool {
			return rm.m.AsTransient().Equal(rm.m.AsTransient())
		},
		genRandomMap,
	))
	properties.Property("new=m.Delete(k) -> new != m", prop.ForAll(
		func(rm *rmap) bool {
			var k string
			rm.m.AsTransient().Range(func(key, val string) bool {
				k = key
				return false
			})
			new := rm.m.Delete(k)
			return !rm.m.AsTransient().Equal(new.AsTransient())
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() != 0
		}),
	))
	properties.Property("m.Equal(10)==false", prop.ForAll(
		func(rm *rmap) bool {
			return !rm.m.AsTransient().Equal(10)
		},
		genRandomMap,
	))
	properties.Property("new=m.Assoc(k,v) -> new != m", prop.ForAll(
		func(rm *rmap) bool {
			var k string
			rm.m.Range(func(key, val string) bool {
				k = key
				return false
			})
			new := rm.m.Assoc(k, "foo")
			return !rm.m.AsTransient().Equal(new.AsTransient())
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() != 0
		}),
	))
	properties.TestingRun(t)
}

func TestTransientRange(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("Range access the full map", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.AsTransient().Range(func(key, val interface{}) bool {
				if !foundAll {
					return false
				}
				foundAll = rm.entries[key.(string)] == val
				return true
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range access the full map no continue", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.AsTransient().Range(func(key, val interface{}) {
				if !foundAll {
					return
				}
				foundAll = rm.entries[key.(string)] == val
				return
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range access the full map entries", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.AsTransient().Range(func(entry Entry) bool {
				if !foundAll {
					return false
				}
				foundAll = rm.entries[entry.Key().(string)] == entry.Value()
				return true
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range access the full map entries no continue", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.AsTransient().Range(func(entry Entry) {
				if !foundAll {
					return
				}
				foundAll = rm.entries[entry.Key().(string)] == entry.Value()
				return
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range with reflected func", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.AsTransient().Range(func(key, val string) bool {
				if !foundAll {
					return false
				}
				foundAll = rm.entries[key] == val
				return true
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range with reflected func no continue", prop.ForAll(
		func(rm *rmap) bool {
			foundAll := true
			rm.m.AsTransient().Range(func(key, val string) {
				if !foundAll {
					return
				}
				foundAll = rm.entries[key] == val
			})

			return foundAll
		},
		genRandomMap,
	))
	properties.Property("Range panics when passed a non function", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.AsTransient().Range(1)
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong number of inputs", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.AsTransient().Range(func(a, b, c string) {})
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong number of outputs", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.AsTransient().Range(func(a, b, c string) (d, e bool) { return false, false })
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong output type", prop.ForAll(
		func(rm *rmap) (ok bool) {
			defer func() {
				r := recover()
				ok = r == errRangeSig
			}()

			rm.m.AsTransient().Range(func(a, b string) string { return "" })
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.Property("Range panics when passed a function with the wrong input types", prop.ForAll(
		func(rm *rmap) (ok bool) {
			ok = true
			defer func() {
				_ = recover()
			}()

			rm.m.AsTransient().Range(func(a, b int) {})
			return false
		},
		genRandomMap.SuchThat(func(rm *rmap) bool {
			return rm.m.Length() > 0
		}),
	))
	properties.TestingRun(t)
	t.Run("Range works with nilable type", func(t *testing.T) {
		defer func() {
			r := recover()
			if r != nil {
				t.Fatal(r)
			}
		}()
		m := Empty().Assoc("a", nil).AsTransient()
		m.Range(func(k string, v *int) {
		})
	})
}

func TestTransientApply(t *testing.T) {
	m := New("a", 1, "b", 2).AsTransient()
	if dyn.Apply(m, "a") != m.At("a") {
		t.Fatal("Apply didn't return the expected value")
	}
}

func makeMap() *Map {
	return Empty()
}

func unmakeMap(m *Map) {
}

var genMap = gopter.DeriveGen(makeMap, unmakeMap)

type lmap struct {
	num  int
	k, v string
	m    *Map
}

func makeLargeMap(num int, k, v string) *lmap {
	m := Empty().AsTransient()
	for i := 0; i < num; i++ {
		m.Assoc(k+strconv.Itoa(i), v+strconv.Itoa(i))
	}
	return &lmap{
		num: num,
		k:   k,
		v:   v,
		m:   m.AsPersistent(),
	}
}

func unmakeLargeMap(lm *lmap) (num int, k, v string) {
	return lm.num, lm.k, lm.v
}

var genLargeMap = gopter.DeriveGen(makeLargeMap, unmakeLargeMap,
	gen.IntRange(1000, 10000),
	gen.Identifier(),
	gen.Identifier(),
)

func makeEntry(key, val string) entry {
	return entry{k: key, v: val}
}

func unmakeEntry(e entry) (key, val string) {
	return e.k.(string), e.v.(string)
}

type rmap struct {
	entries map[string]string
	m       *Map
}

func makeRandomMap(entries map[string]string) *rmap {
	m := Empty().AsTransient()
	for key, val := range entries {
		m = m.Assoc(key, val)
	}
	return &rmap{
		entries: entries,
		m:       m.AsPersistent(),
	}
}

func unmakeRandomMap(r *rmap) map[string]string {
	return r.entries
}

var genRandomMap = gopter.DeriveGen(makeRandomMap, unmakeRandomMap,
	gen.MapOf(gen.Identifier(), gen.Identifier()),
)

func TestSeqString(t *testing.T) {
	out := fmt.Sprint(New("a", "b", "c", "d").Seq())
	if out != "([a b] [c d])" &&
		out != "([c d] [a b])" {
		t.Fatalf("seq.String didn't produce the expected output, got %s",
			out)
	}
}

func TestComparableTypes(t *testing.T) {
	t.Run("keys can be incomparable", func(t *testing.T) {
		key := func() {}
		_ = Empty().Assoc(key, "").Assoc(key, "")
	})
	t.Run("values can be incomparable", func(t *testing.T) {
		val := func() {}
		_ = Empty().Assoc("a", val).Assoc("a", val)
	})
	t.Run("nil values can be incomparable", func(t *testing.T) {
		val := func() {}
		_ = Empty().Assoc("a", nil).Assoc("a", val)
		_ = Empty().Assoc("a", val).Assoc("a", nil)
	})
}

func TestReduce(t *testing.T) {
	// This is a quick test of reduce since the underlying mechanisms
	// are tested thoroughly elsewhere

	t.Run("func(init interface{}, entry Entry) interface{}",
		func(t *testing.T) {
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			out := m.Reduce(func(res interface{}, entry Entry) interface{} {
				return res.(int) + entry.Value().(int)
			}, 0)
			if out != 1+2+3+4+5 {
				t.Fatal("didn't get expected value", out)
			}
		})
	t.Run("func(init interface{}, entry interface{}) interface{}",
		func(t *testing.T) {
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			out := m.Reduce(func(res interface{}, in interface{}) interface{} {
				entry := in.(Entry)
				return res.(int) + entry.Value().(int)
			}, 0)
			if out != 1+2+3+4+5 {
				t.Fatal("didn't get expected value", out)
			}
		})
	t.Run("func(init, k, v interface{}) interface{}",
		func(t *testing.T) {
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			out := m.Reduce(func(res, k, v interface{}) interface{} {
				return res.(int) + v.(int)
			}, 0)
			if out != 1+2+3+4+5 {
				t.Fatal("didn't get expected value", out)
			}
		})
	t.Run("func(init int, e Entry) int",
		func(t *testing.T) {
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			out := m.Reduce(func(res int, e Entry) int {
				return res + e.Value().(int)
			}, 0)
			if out != 1+2+3+4+5 {
				t.Fatal("didn't get expected value", out)
			}
		})
	t.Run("func(init, k, v int) int",
		func(t *testing.T) {
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			out := m.Reduce(func(res, k, v int) int {
				return res + v
			}, 0)
			if out != 1+2+3+4+5 {
				t.Fatal("didn't get expected value", out)
			}
		})
	t.Run("int panics", func(t *testing.T) {
		defer func() {
			r := recover()
			_ = r.(error)
		}()
		m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
		_ = m.Reduce(0, 0)
	})
	t.Run("func(init, k, v int) (int, bool) panics",
		func(t *testing.T) {
			defer func() {
				r := recover()
				_ = r.(error)
			}()
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			_ = m.Reduce(func(res, k, v int) (int, bool) {
				return res + v, true
			}, 0)
		})
	t.Run("func(init, k, v, t int) int panics",
		func(t *testing.T) {
			defer func() {
				r := recover()
				_ = r.(error)
			}()
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5)
			_ = m.Reduce(func(res, k, v, t int) int {
				return res + v
			}, 0)
		})

	t.Run("Transient func(init, k, v int) int",
		func(t *testing.T) {
			m := New(1, 1, 2, 2, 3, 3, 4, 4, 5, 5).AsTransient()
			out := m.Reduce(func(res, k, v int) int {
				return res + v
			}, 0)
			if out != 1+2+3+4+5 {
				t.Fatal("didn't get expected value", out)
			}
		})
}
