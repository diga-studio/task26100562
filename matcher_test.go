package talentpulse

import (
	"math/big"
	"sort"
	"testing"
)

// --- 参考实现：逐候选人独立计算，用于对照 ---

func refEval(f *Filter, p Profile) bool {
	switch f.Op {
	case OpAnd:
		for _, c := range f.Children {
			if !refEval(c, p) {
				return false
			}
		}
		return true
	case OpOr:
		for _, c := range f.Children {
			if refEval(c, p) {
				return true
			}
		}
		return false
	case OpNot:
		return !refEval(f.Children[0], p)
	case OpHasSkill:
		_, ok := p.Skills[f.Skill]
		return ok
	case OpExpRange:
		return p.ExpMonth >= f.MinExp && p.ExpMonth <= f.MaxExp
	}
	return false
}

func refQuery(profiles []Profile, q Query) []ScoredCandidate {
	type hit struct {
		id    string
		score *big.Rat
	}
	var hits []hit
	for _, p := range profiles {
		if _, ok := p.Pools[q.Pool]; !ok {
			continue
		}
		if q.Filter != nil && !refEval(q.Filter, p) {
			continue
		}
		var skillScore int64
		for _, sw := range q.Skills {
			if _, ok := p.Skills[sw.Skill]; ok {
				skillScore += int64(sw.Weight)
			}
		}
		exp := p.ExpMonth
		if exp > q.TargetExp {
			exp = q.TargetExp
		}
		score := new(big.Rat).SetInt64(skillScore)
		score.Add(score, new(big.Rat).SetFrac64(int64(exp)*int64(q.ExpWeight), int64(q.TargetExp)))
		hits = append(hits, hit{id: p.ID, score: score})
	}
	sort.Slice(hits, func(i, j int) bool {
		c := hits[i].score.Cmp(hits[j].score)
		if c != 0 {
			return c > 0
		}
		return hits[i].id < hits[j].id
	})
	if q.K > 0 && len(hits) > q.K {
		hits = hits[:q.K]
	}
	res := make([]ScoredCandidate, len(hits))
	for i, h := range hits {
		f, _ := h.score.Float64()
		res[i] = ScoredCandidate{ID: h.id, Score: f}
	}
	return res
}

func assertSame(t *testing.T, got, want []ScoredCandidate) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("长度不一致: got %d want %d\ngot=%v\nwant=%v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Fatalf("第 %d 名 ID 不一致: got %s want %s\ngot=%v\nwant=%v", i, got[i].ID, want[i].ID, got, want)
		}
		if got[i].Score != want[i].Score {
			t.Fatalf("第 %d 名(%s)分数不一致: got %v want %v", i, got[i].ID, got[i].Score, want[i].Score)
		}
	}
}

func skillSet(skills ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(skills))
	for _, s := range skills {
		m[s] = struct{}{}
	}
	return m
}

func sampleProfiles() []Profile {
	return []Profile{
		{ID: "c01", Skills: skillSet("go", "docker"), ExpMonth: 60, Pools: skillSet("eng")},
		{ID: "c02", Skills: skillSet("go", "k8s"), ExpMonth: 30, Pools: skillSet("eng", "backup")},
		{ID: "c03", Skills: skillSet("java", "spring"), ExpMonth: 6, Pools: skillSet("eng")},
		{ID: "c04", Skills: skillSet("rust"), ExpMonth: 120, Pools: skillSet("eng")},
		{ID: "c05", Skills: skillSet("python"), ExpMonth: 48, Pools: skillSet("eng")},
		{ID: "c06", Skills: skillSet("go"), ExpMonth: 500, Pools: skillSet("eng")},
		{ID: "c07", Skills: skillSet("excel"), ExpMonth: 24, Pools: skillSet("eng")},
		{ID: "c08", Skills: skillSet("go", "java"), ExpMonth: 10, Pools: skillSet("eng")},
		{ID: "c09", Skills: skillSet("go"), ExpMonth: 36, Pools: skillSet("data")},
	}
}

// 嵌套否定 + 多分支过滤 + 无加权技能命中者入选，与逐候选人参考排序一致。
func TestQueryMatchesReference(t *testing.T) {
	m := NewMatcher()
	profiles := sampleProfiles()
	m.AddBatch(profiles)

	// (go OR rust) AND NOT(java AND exp in [0,12])
	filter := &Filter{Op: OpAnd, Children: []*Filter{
		{Op: OpOr, Children: []*Filter{
			{Op: OpHasSkill, Skill: "go"},
			{Op: OpHasSkill, Skill: "rust"},
		}},
		{Op: OpNot, Children: []*Filter{
			{Op: OpAnd, Children: []*Filter{
				{Op: OpHasSkill, Skill: "java"},
				{Op: OpExpRange, MinExp: 0, MaxExp: 12},
			}},
		}},
	}}
	q := Query{
		Pool:      "eng",
		Filter:    filter,
		Skills:    []SkillWeight{{Skill: "go", Weight: 700}, {Skill: "k8s", Weight: 300}},
		TargetExp: 48,
		ExpWeight: 200,
		K:         10,
	}
	assertSame(t, m.Query(q), refQuery(profiles, q))

	// 无加权技能命中但仍通过过滤的候选人必须入选（c04 rust 无权重、经验分仍计入）。
	qNoSkillHit := Query{
		Pool:      "eng",
		Filter:    &Filter{Op: OpHasSkill, Skill: "rust"},
		Skills:    []SkillWeight{{Skill: "go", Weight: 500}},
		TargetExp: 48,
		ExpWeight: 400,
		K:         5,
	}
	got := m.Query(qNoSkillHit)
	assertSame(t, got, refQuery(profiles, qNoSkillHit))
	if len(got) != 1 || got[0].ID != "c04" {
		t.Fatalf("无加权技能命中者未正确入选: %v", got)
	}

	// K 小于命中数时只取前 K；K 大于命中数时全部返回。
	qTop2 := q
	qTop2.K = 2
	assertSame(t, m.Query(qTop2), refQuery(profiles, qTop2))

	// 不同人才池互不影响：data 池只有 c09。
	qData := Query{Pool: "data", Skills: q.Skills, TargetExp: 48, ExpWeight: 200, K: 10}
	assertSame(t, m.Query(qData), refQuery(profiles, qData))
}

// 整份替换技能、经验、人才池后，结果与按新档案重新计算完全一致。
func TestReplaceTakesEffect(t *testing.T) {
	m := NewMatcher()
	profiles := sampleProfiles()
	m.AddBatch(profiles)

	// c01: 换掉全部技能与经验；c03: 换池到 data；c07: 新增 go 技能与经验。
	m.Replace(Profile{ID: "c01", Skills: skillSet("python"), ExpMonth: 12, Pools: skillSet("eng")})
	m.Replace(Profile{ID: "c03", Skills: skillSet("go"), ExpMonth: 90, Pools: skillSet("data")})
	m.Replace(Profile{ID: "c07", Skills: skillSet("go"), ExpMonth: 200, Pools: skillSet("eng")})

	updated := []Profile{
		{ID: "c01", Skills: skillSet("python"), ExpMonth: 12, Pools: skillSet("eng")},
		{ID: "c02", Skills: skillSet("go", "k8s"), ExpMonth: 30, Pools: skillSet("eng", "backup")},
		{ID: "c03", Skills: skillSet("go"), ExpMonth: 90, Pools: skillSet("data")},
		{ID: "c04", Skills: skillSet("rust"), ExpMonth: 120, Pools: skillSet("eng")},
		{ID: "c05", Skills: skillSet("python"), ExpMonth: 48, Pools: skillSet("eng")},
		{ID: "c06", Skills: skillSet("go"), ExpMonth: 500, Pools: skillSet("eng")},
		{ID: "c07", Skills: skillSet("go"), ExpMonth: 200, Pools: skillSet("eng")},
		{ID: "c08", Skills: skillSet("go", "java"), ExpMonth: 10, Pools: skillSet("eng")},
		{ID: "c09", Skills: skillSet("go"), ExpMonth: 36, Pools: skillSet("data")},
	}

	q := Query{
		Pool:      "eng",
		Filter:    &Filter{Op: OpHasSkill, Skill: "go"},
		Skills:    []SkillWeight{{Skill: "go", Weight: 600}, {Skill: "docker", Weight: 400}},
		TargetExp: 60,
		ExpWeight: 300,
		K:         20,
	}
	got := m.Query(q)
	assertSame(t, got, refQuery(updated, q))

	// c01 旧技能 docker 不再命中，旧档案不得再被召回。
	for _, sc := range got {
		if sc.ID == "c01" {
			t.Fatalf("c01 替换后仍命中旧技能 docker: %v", got)
		}
	}

	// c03 已离开 eng 池，不能再出现在 eng 查询中；但应出现在 data 池中。
	for _, sc := range got {
		if sc.ID == "c03" {
			t.Fatalf("c03 替换池后仍命中 eng: %v", got)
		}
	}
	qData := Query{Pool: "data", Filter: q.Filter, Skills: q.Skills, TargetExp: 60, ExpWeight: 300, K: 20}
	assertSame(t, m.Query(qData), refQuery(updated, qData))

	// 旧人才池查询：backup 池仅 c02，替换不影响未替换者。
	qBackup := Query{Pool: "backup", Skills: q.Skills, TargetExp: 60, ExpWeight: 300, K: 20}
	assertSame(t, m.Query(qBackup), refQuery(updated, qBackup))
}

// 同分按 ID 升序；分数仅差 1/480 时名次不被浮点截断翻转。
func TestExactOrderingAndTieBreak(t *testing.T) {
	m := NewMatcher()
	profiles := []Profile{
		// 同分：skill 100 + exp 满分 200*1 = 300
		{ID: "b2", Skills: skillSet("a"), ExpMonth: 480, Pools: skillSet("p")},
		{ID: "b1", Skills: skillSet("a"), ExpMonth: 999, Pools: skillSet("p")},
		// 仅差 1 个月经验：总分差 200/480，排在同分者之后
		{ID: "b3", Skills: skillSet("a"), ExpMonth: 479, Pools: skillSet("p")},
		// 技能分高 1，经验少 1 个月：100+1 技能 vs 经验差 200/480，总分更高
		{ID: "b0", Skills: skillSet("a", "b"), ExpMonth: 479, Pools: skillSet("p")},
	}
	m.AddBatch(profiles)
	q := Query{
		Pool:      "p",
		Skills:    []SkillWeight{{Skill: "a", Weight: 100}, {Skill: "b", Weight: 1}},
		TargetExp: 480,
		ExpWeight: 200,
		K:         10,
	}
	got := m.Query(q)
	assertSame(t, got, refQuery(profiles, q))
	wantOrder := []string{"b0", "b1", "b2", "b3"}
	if len(got) != len(wantOrder) {
		t.Fatalf("命中数不符: %v", got)
	}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("第 %d 名应为 %s，实际 %v", i, id, got)
		}
	}
}
