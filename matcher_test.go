package talentpulse

import (
	"reflect"
	"sort"
	"testing"
)

// refEval 参考实现：对单个候选人独立求值过滤条件。
func refEval(f *Filter, c Candidate) bool {
	has := func(s string) bool {
		for _, x := range c.Skills {
			if x == s {
				return true
			}
		}
		return false
	}
	switch f.Op {
	case OpSkill:
		return has(f.Skill)
	case OpExp:
		return c.ExperienceMths >= f.Min && c.ExperienceMths <= f.Max
	case OpAnd:
		for _, ch := range f.Childs {
			if !refEval(ch, c) {
				return false
			}
		}
		return true
	case OpOr:
		for _, ch := range f.Childs {
			if refEval(ch, c) {
				return true
			}
		}
		return false
	case OpNot:
		return !refEval(f.Child, c)
	}
	return false
}

// refQuery 参考实现：逐候选人计算完整排序（精确有理数比较）。
func refQuery(all []Candidate, q Query) []ScoredCandidate {
	w := map[string]int64{}
	for _, ws := range q.Skills {
		w[ws.Skill] = int64(ws.Weight)
	}
	target := int64(q.TargetExpMth)
	type item struct {
		sc  ScoredCandidate
		key int64
	}
	var items []item
	for _, c := range all {
		inPool := false
		for _, p := range c.Pools {
			if p == q.Pool {
				inPool = true
			}
		}
		if !inPool {
			continue
		}
		if q.Filter != nil && !refEval(q.Filter, c) {
			continue
		}
		seen := map[string]bool{}
		var skillScore int64
		for _, s := range c.Skills {
			if !seen[s] {
				seen[s] = true
				skillScore += w[s]
			}
		}
		me := int64(c.ExperienceMths)
		if me > target {
			me = target
		}
		key := skillScore*target + int64(q.ExpWeight)*me
		items = append(items, item{
			sc:  ScoredCandidate{Candidate: c, Score: float64(key) / float64(target)},
			key: key,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].key != items[j].key {
			return items[i].key > items[j].key
		}
		return items[i].sc.Candidate.ID < items[j].sc.Candidate.ID
	})
	if q.K >= 0 && len(items) > q.K {
		items = items[:q.K]
	}
	out := make([]ScoredCandidate, len(items))
	for i, it := range items {
		out[i] = it.sc
	}
	return out
}

func assertQuery(t *testing.T, m *Matcher, all []Candidate, q Query) {
	t.Helper()
	got := m.Query(q)
	want := refQuery(all, q)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query %+v\ngot:  %+v\nwant: %+v", q, got, want)
	}
}

func sampleCandidates() []Candidate {
	return []Candidate{
		{ID: "c01", Skills: []string{"go", "sql"}, ExperienceMths: 24, Pools: []string{"eng"}},
		{ID: "c02", Skills: []string{"go", "k8s"}, ExperienceMths: 60, Pools: []string{"eng", "senior"}},
		{ID: "c03", Skills: []string{"java"}, ExperienceMths: 12, Pools: []string{"eng"}},
		{ID: "c04", Skills: []string{"sql", "python"}, ExperienceMths: 36, Pools: []string{"data"}},
		{ID: "c05", Skills: []string{"go"}, ExperienceMths: 480, Pools: []string{"eng", "senior"}},
		{ID: "c06", Skills: []string{"rust"}, ExperienceMths: 6, Pools: []string{"eng"}},
		{ID: "c07", Skills: []string{"go", "go", "sql"}, ExperienceMths: 18, Pools: []string{"eng"}},
		{ID: "c08", Skills: []string{}, ExperienceMths: 30, Pools: []string{"eng"}},
	}
}

func TestQueryMatchesReference(t *testing.T) {
	all := sampleCandidates()
	m := NewMatcher()
	m.Add(all)

	nestedNot := &Filter{Op: OpNot, Child: &Filter{
		Op: OpOr,
		Childs: []*Filter{
			{Op: OpSkill, Skill: "java"},
			{Op: OpAnd, Childs: []*Filter{
				{Op: OpSkill, Skill: "go"},
				{Op: OpNot, Child: &Filter{Op: OpExp, Min: 0, Max: 24}},
			}},
		},
	}}
	queries := []Query{
		// 嵌套否定 + 加权技能
		{Pool: "eng", Filter: nestedNot, Skills: []WeightedSkill{{"go", 500}, {"sql", 300}}, TargetExpMth: 48, ExpWeight: 200, K: 10},
		// 不同过滤分支：纯经验区间
		{Pool: "eng", Filter: &Filter{Op: OpExp, Min: 12, Max: 36}, Skills: []WeightedSkill{{"go", 100}}, TargetExpMth: 24, ExpWeight: 1000, K: 3},
		// 无过滤，包含无加权技能命中的入选者（rust / 无技能）
		{Pool: "eng", Filter: nil, Skills: []WeightedSkill{{"go", 700}}, TargetExpMth: 100, ExpWeight: 50, K: 100},
		// OR 分支 + K 截断
		{Pool: "eng", Filter: &Filter{Op: OpOr, Childs: []*Filter{{Op: OpSkill, Skill: "sql"}, {Op: OpSkill, Skill: "k8s"}}}, Skills: []WeightedSkill{{"sql", 250}, {"k8s", 250}}, TargetExpMth: 480, ExpWeight: 0, K: 1},
		// 人才池隔离：data 池
		{Pool: "data", Filter: nil, Skills: []WeightedSkill{{"sql", 900}, {"python", 900}}, TargetExpMth: 36, ExpWeight: 400, K: 10},
		// NOT 叶子直接取反
		{Pool: "eng", Filter: &Filter{Op: OpNot, Child: &Filter{Op: OpSkill, Skill: "go"}}, Skills: []WeightedSkill{{"go", 1000}}, TargetExpMth: 60, ExpWeight: 500, K: 10},
		// 超过目标经验不再加分：c05(480月) 与目标 24 比较
		{Pool: "senior", Filter: nil, Skills: []WeightedSkill{{"go", 0}}, TargetExpMth: 24, ExpWeight: 1000, K: 10},
	}
	for _, q := range queries {
		assertQuery(t, m, all, q)
	}
}

func TestReplaceTakesEffect(t *testing.T) {
	all := sampleCandidates()
	m := NewMatcher()
	m.Add(all)

	// 整份替换：改技能、经验、人才池
	replacements := []Candidate{
		{ID: "c03", Skills: []string{"go", "sql", "k8s"}, ExperienceMths: 120, Pools: []string{"eng", "senior"}},
		{ID: "c01", Skills: []string{"java"}, ExperienceMths: 3, Pools: []string{"data"}}, // 移出 eng 池
		{ID: "c06", Skills: []string{}, ExperienceMths: 6, Pools: []string{}},            // 不再属于任何池
	}
	for _, r := range replacements {
		m.Replace(r)
	}
	updated := make([]Candidate, len(all))
	copy(updated, all)
	for i, c := range updated {
		for _, r := range replacements {
			if c.ID == r.ID {
				updated[i] = r
			}
		}
	}

	queries := []Query{
		// 旧技能不再命中：c01 已无 go/sql；c03 新技能应命中
		{Pool: "eng", Filter: &Filter{Op: OpSkill, Skill: "go"}, Skills: []WeightedSkill{{"go", 500}, {"sql", 300}, {"k8s", 400}}, TargetExpMth: 60, ExpWeight: 300, K: 10},
		// 旧人才池不再命中：c01 已离开 eng，c06 不在任何池
		{Pool: "eng", Filter: nil, Skills: []WeightedSkill{{"go", 100}}, TargetExpMth: 48, ExpWeight: 100, K: 100},
		// 新人才池命中：c01 现在属于 data
		{Pool: "data", Filter: nil, Skills: []WeightedSkill{{"java", 800}}, TargetExpMth: 12, ExpWeight: 600, K: 10},
		// 嵌套过滤 + senior 池新成员 c03
		{Pool: "senior", Filter: &Filter{Op: OpNot, Child: &Filter{Op: OpExp, Min: 0, Max: 59}}, Skills: []WeightedSkill{{"go", 600}}, TargetExpMth: 100, ExpWeight: 100, K: 10},
	}
	for _, q := range queries {
		assertQuery(t, m, updated, q)
	}
}

func TestExactOrderingNoTruncation(t *testing.T) {
	// 构造分值仅在分数部分有细微差异的候选人，验证精确排序。
	all := []Candidate{
		{ID: "a", Skills: []string{}, ExperienceMths: 1, Pools: []string{"p"}},
		{ID: "b", Skills: []string{}, ExperienceMths: 2, Pools: []string{"p"}},
		{ID: "c", Skills: []string{"x"}, ExperienceMths: 0, Pools: []string{"p"}},
	}
	m := NewMatcher()
	m.Add(all)
	q := Query{Pool: "p", Skills: []WeightedSkill{{"x", 1}}, TargetExpMth: 3, ExpWeight: 1, K: 10}
	// a=1/3, b=2/3, c=1；期望 c,b,a
	got := m.Query(q)
	want := refQuery(all, q)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	ids := []string{got[0].Candidate.ID, got[1].Candidate.ID, got[2].Candidate.ID}
	if !reflect.DeepEqual(ids, []string{"c", "b", "a"}) {
		t.Fatalf("order %v", ids)
	}
}
