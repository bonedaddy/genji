package query_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/asdine/genji/engine/bolt"
	"github.com/asdine/genji/field"
	"github.com/asdine/genji/index"
	"github.com/asdine/genji/query"
	"github.com/asdine/genji/record"
	bbolt "github.com/etcd-io/bbolt"
	"github.com/stretchr/testify/require"
)

func createRecord(age int) record.Record {
	var fb record.FieldBuffer

	fb.Add(field.NewInt64("age", int64(age)))

	return &fb
}

func TestMatchers(t *testing.T) {
	type matcher interface {
		Match(record.Record) (bool, error)
	}

	r := createRecord(10)
	tests := []struct {
		name    string
		matcher matcher
		match   bool
	}{
		{"eq", query.EqInt(query.Field("age"), 10), true},
		{"gt/10>10", query.GtInt(query.Field("age"), 10), false},
		{"gt/10>11", query.GtInt(query.Field("age"), 11), false},
		{"gt/10>9", query.GtInt(query.Field("age"), 9), true},
		{"gte/10>=10", query.GteInt(query.Field("age"), 10), true},
		{"gte/10>=11", query.GteInt(query.Field("age"), 11), false},
		{"gte/10>=9", query.GteInt(query.Field("age"), 9), true},
		{"lt/10<10", query.LtInt(query.Field("age"), 10), false},
		{"lt/10<11", query.LtInt(query.Field("age"), 11), true},
		{"lt/10<9", query.LtInt(query.Field("age"), 9), false},
		{"lte/10<=10", query.LteInt(query.Field("age"), 10), true},
		{"lte/10<=11", query.LteInt(query.Field("age"), 11), true},
		{"lte/10<=9", query.LteInt(query.Field("age"), 9), false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched, err := test.matcher.Match(r)
			require.NoError(t, err)
			require.Equal(t, test.match, matched)
		})
	}
}

func createIndexMap(t require.TestingT, ages []int, teams []string) (map[string]index.Index, func()) {
	dir, err := ioutil.TempDir("", "genji")
	require.NoError(t, err)

	db, err := bbolt.Open(path.Join(dir, "test.db"), 0600, nil)
	require.NoError(t, err)

	tx, err := db.Begin(true)
	require.NoError(t, err)

	indexes := make(map[string]index.Index)
	indexes["age"] = createIntIndex(t, tx, ages)
	indexes["team"] = createStrIndex(t, tx, teams)

	return indexes, func() {
		tx.Rollback()
		db.Close()
		os.RemoveAll(dir)
	}
}

func createIntIndex(t require.TestingT, tx *bbolt.Tx, ages []int) index.Index {
	b, err := tx.CreateBucket([]byte("age"))
	require.NoError(t, err)

	idx := bolt.NewIndex(b)

	for i, age := range ages {
		err := idx.Set(field.EncodeInt64(int64(age)), field.EncodeInt64(int64(i)))
		require.NoError(t, err)
	}

	return idx
}

func createStrIndex(t require.TestingT, tx *bbolt.Tx, teams []string) index.Index {
	b, err := tx.CreateBucket([]byte("team"))
	require.NoError(t, err)

	idx := bolt.NewIndex(b)

	for i, team := range teams {
		err := idx.Set([]byte(team), field.EncodeInt64(int64(i)))
		require.NoError(t, err)
	}

	return idx
}

func TestIndexMatchers(t *testing.T) {
	type indexMatcher interface {
		MatchIndex(im map[string]index.Index) ([][]byte, error)
	}

	im, cleanup := createIndexMap(t, []int{1, 2, 2, 3, 5, 10}, []string{"ACA", "LOSC", "OL", "OM", "OM", "PSG"})
	defer cleanup()

	tests := []struct {
		name    string
		matcher indexMatcher
		rowids  []int64
	}{
		{"eq/int/one", query.EqInt(query.Field("age"), 10), []int64{5}},
		{"eq/int/multiple", query.EqInt(query.Field("age"), 2), []int64{1, 2}},
		{"eq/int/none", query.EqInt(query.Field("age"), 15), nil},
		{"eq/str/one", query.EqStr(query.Field("team"), "PSG"), []int64{5}},
		{"eq/str/multiple", query.EqStr(query.Field("team"), "OM"), []int64{3, 4}},
		{"eq/str/none", query.EqStr(query.Field("team"), "SCB"), nil},
		{"gt/int/>10", query.GtInt(query.Field("age"), 10), nil},
		{"gt/int/>7", query.GtInt(query.Field("age"), 7), []int64{5}},
		{"gt/int/>1", query.GtInt(query.Field("age"), 1), []int64{1, 2, 3, 4, 5}},
		{"gt/int/>-1", query.GtInt(query.Field("age"), -1), []int64{0, 1, 2, 3, 4, 5}},
		{"gt/str/>PSG", query.GtStr(query.Field("team"), "PSG"), nil},
		{"gt/str/>OM", query.GtStr(query.Field("team"), "OM"), []int64{5}},
		{"gt/str/>NICE", query.GtStr(query.Field("team"), "NICE"), []int64{2, 3, 4, 5}},
		{"gt/str/>ACA", query.GtStr(query.Field("team"), "ACA"), []int64{1, 2, 3, 4, 5}},
		{"gt/str/>A", query.GtStr(query.Field("team"), "A"), []int64{0, 1, 2, 3, 4, 5}},
		{"gte/int/>=11", query.GteInt(query.Field("age"), 11), nil},
		{"gte/int/>=7", query.GteInt(query.Field("age"), 7), []int64{5}},
		{"gte/int/>=2", query.GteInt(query.Field("age"), 2), []int64{1, 2, 3, 4, 5}},
		{"gte/int/>=1", query.GteInt(query.Field("age"), 1), []int64{0, 1, 2, 3, 4, 5}},
		{"gte/str/>=PSG", query.GteStr(query.Field("team"), "PSG"), []int64{5}},
		{"gte/str/>=OM", query.GteStr(query.Field("team"), "OM"), []int64{3, 4, 5}},
		{"gte/str/>=NICE", query.GteStr(query.Field("team"), "NICE"), []int64{2, 3, 4, 5}},
		{"gte/str/>=ACA", query.GteStr(query.Field("team"), "ACA"), []int64{0, 1, 2, 3, 4, 5}},
		{"lt/int/<1", query.LtInt(query.Field("age"), 1), nil},
		{"lt/int/<4", query.LtInt(query.Field("age"), 4), []int64{0, 1, 2, 3}},
		{"lt/int/<10", query.LtInt(query.Field("age"), 10), []int64{0, 1, 2, 3, 4}},
		{"lt/int/<11", query.LtInt(query.Field("age"), 11), []int64{0, 1, 2, 3, 4, 5}},
		{"lt/str/<A", query.LtStr(query.Field("team"), "A"), nil},
		{"lt/str/<ACA", query.LtStr(query.Field("team"), "ACA"), nil},
		{"lt/str/<NICE", query.LtStr(query.Field("team"), "NICE"), []int64{0, 1}},
		{"lt/str/<OM", query.LtStr(query.Field("team"), "OM"), []int64{0, 1, 2}},
		{"lt/str/<STRASBOURG", query.LtStr(query.Field("team"), "STRASBOURG"), []int64{0, 1, 2, 3, 4, 5}},
		{"lte/int/<=0", query.LteInt(query.Field("age"), 0), nil},
		{"lte/int/<=2", query.LteInt(query.Field("age"), 2), []int64{0, 1, 2}},
		{"lte/int/<=4", query.LteInt(query.Field("age"), 4), []int64{0, 1, 2, 3}},
		{"lte/int/<=10", query.LteInt(query.Field("age"), 10), []int64{0, 1, 2, 3, 4, 5}},
		{"lte/int/<=11", query.LteInt(query.Field("age"), 11), []int64{0, 1, 2, 3, 4, 5}},
		{"lte/str/<=A", query.LteStr(query.Field("team"), "A"), nil},
		{"lte/str/<=ACA", query.LteStr(query.Field("team"), "ACA"), []int64{0}},
		{"lte/str/<=NICE", query.LteStr(query.Field("team"), "NICE"), []int64{0, 1}},
		{"lte/str/<=OM", query.LteStr(query.Field("team"), "OM"), []int64{0, 1, 2, 3, 4}},
		{"lte/str/<=STRASBOURG", query.LteStr(query.Field("team"), "STRASBOURG"), []int64{0, 1, 2, 3, 4, 5}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rowids, err := test.matcher.MatchIndex(im)
			require.NoError(t, err)
			var ids []int64
			for _, rowid := range rowids {
				id, err := field.DecodeInt64(rowid)
				require.NoError(t, err)
				ids = append(ids, id)
			}
			require.EqualValues(t, test.rowids, ids)
		})
	}
}

type simpleMatcher struct{}

func (s *simpleMatcher) Match(record.Record) (bool, error) {
	return true, nil
}

func TestAndMatcher(t *testing.T) {
	t.Run("Matcher", func(t *testing.T) {
		m := query.And(
			query.GtInt(query.Field("age"), 2),
			query.LtInt(query.Field("age"), 10),
		)

		ok, err := m.Match(createRecord(5))
		require.NoError(t, err)
		require.True(t, ok)

		ok, err = m.Match(createRecord(10))
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("IndexMatcher", func(t *testing.T) {
		im, cleanup := createIndexMap(t, []int{1, 2, 2, 3, 5, 10}, []string{"ACA", "LOSC", "OL", "OM", "OM", "PSG"})
		defer cleanup()

		tests := []struct {
			name     string
			matchers []query.Matcher
			expected []int64
		}{
			{">2", []query.Matcher{query.GtInt(query.Field("age"), 2)}, []int64{3, 4, 5}},
			{">2 && <10", []query.Matcher{query.GtInt(query.Field("age"), 2), query.LtInt(query.Field("age"), 10)}, []int64{3, 4}},
			{">10 && <20", []query.Matcher{query.GtInt(query.Field("age"), 10), query.LtInt(query.Field("age"), 20)}, []int64{}},
			{">8 && <3", []query.Matcher{query.GtInt(query.Field("age"), 8), query.LtInt(query.Field("age"), 3)}, []int64{}},
			{">8 && non index matcher", []query.Matcher{query.GtInt(query.Field("age"), 8), new(simpleMatcher)}, []int64{}},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				m := query.And(test.matchers...)

				rowids, err := m.MatchIndex(im)
				require.NoError(t, err)

				ids := make([]int64, len(rowids))
				for i, rowid := range rowids {
					id, err := field.DecodeInt64(rowid)
					require.NoError(t, err)
					ids[i] = id
				}

				require.Equal(t, test.expected, ids)
			})
		}

	})
}

func TestOrMatcher(t *testing.T) {
	t.Run("Matcher", func(t *testing.T) {
		m := query.Or(
			query.GtInt(query.Field("age"), 8),
			query.LtInt(query.Field("age"), 2),
		)

		ok, err := m.Match(createRecord(1))
		require.NoError(t, err)
		require.True(t, ok)

		ok, err = m.Match(createRecord(9))
		require.NoError(t, err)
		require.True(t, ok)

		ok, err = m.Match(createRecord(5))
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("IndexMatcher", func(t *testing.T) {
		im, cleanup := createIndexMap(t, []int{1, 2, 2, 3, 5, 10}, []string{"ACA", "LOSC", "OL", "OM", "OM", "PSG"})
		defer cleanup()

		tests := []struct {
			name     string
			matchers []query.Matcher
			expected []int64
		}{
			{">2", []query.Matcher{query.GtInt(query.Field("age"), 2)}, []int64{3, 4, 5}},
			{">8 || <2", []query.Matcher{query.GtInt(query.Field("age"), 8), query.LtInt(query.Field("age"), 2)}, []int64{5, 0}},
			{">0 || <11", []query.Matcher{query.GtInt(query.Field("age"), 0), query.LtInt(query.Field("age"), 11)}, []int64{0, 1, 2, 3, 4, 5}},
			{">10 || <20", []query.Matcher{query.GtInt(query.Field("age"), 10), query.LtInt(query.Field("age"), 20)}, []int64{0, 1, 2, 3, 4, 5}},
			{">10 || >20", []query.Matcher{query.GtInt(query.Field("age"), 10), query.GtInt(query.Field("age"), 20)}, []int64{}},
			{">8 || non index matcher", []query.Matcher{query.GtInt(query.Field("age"), 8), new(simpleMatcher)}, []int64{}},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				m := query.Or(test.matchers...)

				rowids, err := m.MatchIndex(im)
				require.NoError(t, err)

				ids := make([]int64, len(rowids))
				for i, rowid := range rowids {
					id, err := field.DecodeInt64(rowid)
					require.NoError(t, err)
					ids[i] = id
				}

				require.Equal(t, test.expected, ids)
			})
		}

	})
}

func benchmarkMatcher(b *testing.B, size int) {
	records := make([]record.Record, size)
	for i := 0; i < size; i++ {
		records[i] = createRecord(i)
	}

	matcher := query.And(
		query.GtInt(
			query.Field("age"),
			2,
		),
		query.LtInt(
			query.Field("age"),
			10,
		),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, r := range records {
			matcher.Match(r)
		}
	}
}

func BenchmarkMatcher1(b *testing.B) {
	benchmarkMatcher(b, 1)
}

func BenchmarkMatcher10(b *testing.B) {
	benchmarkMatcher(b, 10)
}

func BenchmarkMatcher100(b *testing.B) {
	benchmarkMatcher(b, 100)
}

func BenchmarkMatcher1000(b *testing.B) {
	benchmarkMatcher(b, 1000)
}

func BenchmarkMatcher10000(b *testing.B) {
	benchmarkMatcher(b, 10000)
}

func benchmarkIndexMatcher(b *testing.B, size int) {
	ages := make([]int, size)
	for i := 0; i < size; i++ {
		ages[i] = i
	}

	im, cleanup := createIndexMap(b, ages, nil)
	defer cleanup()

	matcher := query.And(
		query.GtInt(
			query.Field("age"),
			2,
		),
		query.LtInt(
			query.Field("age"),
			10,
		),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.MatchIndex(im)
	}
	b.StopTimer()
}

func BenchmarkIndexMatcher1(b *testing.B) {
	benchmarkIndexMatcher(b, 1)
}
func BenchmarkIndexMatcher10(b *testing.B) {
	benchmarkIndexMatcher(b, 10)
}

func BenchmarkIndexMatcher100(b *testing.B) {
	benchmarkIndexMatcher(b, 100)
}

func BenchmarkIndexMatcher1000(b *testing.B) {
	benchmarkIndexMatcher(b, 1000)
}

func BenchmarkIndexMatcher10000(b *testing.B) {
	benchmarkIndexMatcher(b, 10000)
}