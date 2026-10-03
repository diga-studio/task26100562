package talentpulse

import "sort"

// Matcher 候选人匹配库，档案保存在进程内存。
type Matcher struct {
	candidates map[string]Candidate
}

// NewMatcher 创建空的匹配库。
func NewMatcher() *Matcher {
	return &Matcher{candidates: make(map[string]Candidate)}
}

// Add 批量录入候选人；ID 已存在时整份覆盖。
func (m *Matcher) Add(cands []Candidate) {
	for _, c := range cands {
		m.candidates[c.ID] = c
	}
}

// Replace 按 ID 整份替换档案；替换完成后所有后续查询只依据新档案。
func (m *Matcher) Replace(c Candidate) {
	m.candidates[c.ID] = c
}

// matchFilter 判断候选人是否满足过滤条件。
// NOT 的全集即所选人才池内的当前候选人，因此对池内成员直接取反即可。
func matchFilter(f *Filter, skills map[string]struct{}, expMths int) bool {
	switch f.Op {
	case OpSkill:
		_, ok := skills[f.Skill]
		return ok
	case OpExp:
		return expMths >= f.Min && expMths <= f.Max
	case OpAnd:
		for _, c := range f.Childs {
			if !matchFilter(c, skills, expMths) {
				return false
			}
		}
		return true
	case OpOr:
		for _, c := range f.Childs {
			if matchFilter(c, skills, expMths) {
				return true
			}
		}
		return false
	case OpNot:
		return !matchFilter(f.Child, skills, expMths)
	}
	return false
}

// Query 返回满足过滤条件的前 K 名，按总分降序、候选人 ID 升序排列。
// 排序依据精确分值：总分 = 技能得分 + 经验权重*min(经验,目标)/目标，
// 用整数 key = 技能得分*目标 + 经验权重*min(经验,目标) 比较，避免小数截断改变名次。
func (m *Matcher) Query(q Query) []ScoredCandidate {
	weights := make(map[string]int64, len(q.Skills))
	for _, ws := range q.Skills {
		weights[ws.Skill] = int64(ws.Weight)
	}
	target := int64(q.TargetExpMth)
	if target <= 0 {
		target = 1
	}

	type hit struct {
		cand ScoredCandidate
		key  int64
	}
	var hits []hit
	for _, c := range m.candidates {
		inPool := false
		for _, p := range c.Pools {
			if p == q.Pool {
				inPool = true
				break
			}
		}
		if !inPool {
			continue
		}
		skillSet := make(map[string]struct{}, len(c.Skills))
		for _, s := range c.Skills {
			skillSet[s] = struct{}{}
		}
		if q.Filter != nil && !matchFilter(q.Filter, skillSet, c.ExperienceMths) {
			continue
		}
		var skillScore int64
		for s := range skillSet {
			skillScore += weights[s]
		}
		minExp := int64(c.ExperienceMths)
		if minExp > target {
			minExp = target
		}
		key := skillScore*target + int64(q.ExpWeight)*minExp
		hits = append(hits, hit{
			cand: ScoredCandidate{
				Candidate: c,
				Score:     float64(key) / float64(target),
			},
			key: key,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].key != hits[j].key {
			return hits[i].key > hits[j].key
		}
		return hits[i].cand.Candidate.ID < hits[j].cand.Candidate.ID
	})
	if q.K >= 0 && len(hits) > q.K {
		hits = hits[:q.K]
	}
	out := make([]ScoredCandidate, len(hits))
	for i, h := range hits {
		out[i] = h.cand
	}
	return out
}
