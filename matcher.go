package talentpulse

import "sort"

// Matcher 候选人匹配库，全部数据保存在进程内存。
type Matcher struct {
	profiles map[string]*Profile
}

// NewMatcher 创建空的匹配库。
func NewMatcher() *Matcher {
	return &Matcher{profiles: make(map[string]*Profile)}
}

func copyProfile(p Profile) *Profile {
	skills := make(map[string]struct{}, len(p.Skills))
	for s := range p.Skills {
		skills[s] = struct{}{}
	}
	pools := make(map[string]struct{}, len(p.Pools))
	for pl := range p.Pools {
		pools[pl] = struct{}{}
	}
	return &Profile{ID: p.ID, Skills: skills, ExpMonth: p.ExpMonth, Pools: pools}
}

// AddBatch 批量录入候选人。ID 已存在时整份替换。
func (m *Matcher) AddBatch(profiles []Profile) {
	for i := range profiles {
		m.profiles[profiles[i].ID] = copyProfile(profiles[i])
	}
}

// Replace 按 ID 整份替换档案；不存在则新增。
func (m *Matcher) Replace(p Profile) {
	m.profiles[p.ID] = copyProfile(p)
}

// evalFilter 判断候选人是否满足过滤条件。
// NOT 的全集即所选人才池内的当前候选人，因此只需对池内候选人求值。
func evalFilter(f *Filter, p *Profile) bool {
	switch f.Op {
	case OpAnd:
		for _, c := range f.Children {
			if !evalFilter(c, p) {
				return false
			}
		}
		return true
	case OpOr:
		for _, c := range f.Children {
			if evalFilter(c, p) {
				return true
			}
		}
		return false
	case OpNot:
		if len(f.Children) == 0 {
			return true
		}
		return !evalFilter(f.Children[0], p)
	case OpHasSkill:
		_, ok := p.Skills[f.Skill]
		return ok
	case OpExpRange:
		return p.ExpMonth >= f.MinExp && p.ExpMonth <= f.MaxExp
	}
	return false
}

// scoreNum 返回总分的精确分子：总分 = 分子 / TargetExp。
// 技能分为整数，经验分 = min(exp,target)/target*expWeight，
// 同一次查询 TargetExp 固定，比较分子即精确比较总分。
func scoreNum(p *Profile, weights map[string]int, targetExp, expWeight int) int64 {
	var skillScore int64
	for s, w := range weights {
		if _, ok := p.Skills[s]; ok {
			skillScore += int64(w)
		}
	}
	exp := p.ExpMonth
	if exp > targetExp {
		exp = targetExp
	}
	return skillScore*int64(targetExp) + int64(expWeight)*int64(exp)
}

// Query 返回满足过滤条件的前 K 名，按总分降序、ID 升序。
func (m *Matcher) Query(q Query) []ScoredCandidate {
	weights := make(map[string]int, len(q.Skills))
	for _, sw := range q.Skills {
		weights[sw.Skill] = sw.Weight
	}
	type hit struct {
		id  string
		num int64
	}
	var hits []hit
	for _, p := range m.profiles {
		if _, ok := p.Pools[q.Pool]; !ok {
			continue
		}
		if q.Filter != nil && !evalFilter(q.Filter, p) {
			continue
		}
		hits = append(hits, hit{id: p.ID, num: scoreNum(p, weights, q.TargetExp, q.ExpWeight)})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].num != hits[j].num {
			return hits[i].num > hits[j].num
		}
		return hits[i].id < hits[j].id
	})
	if q.K > 0 && len(hits) > q.K {
		hits = hits[:q.K]
	}
	res := make([]ScoredCandidate, len(hits))
	for i, h := range hits {
		res[i] = ScoredCandidate{ID: h.id, Score: float64(h.num) / float64(q.TargetExp)}
	}
	return res
}
