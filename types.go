package talentpulse

// Profile 候选人档案。
type Profile struct {
	ID       string
	Skills   map[string]struct{}
	ExpMonth int
	Pools    map[string]struct{}
}

// FilterOp 过滤条件节点类型。
type FilterOp int

const (
	OpAnd FilterOp = iota
	OpOr
	OpNot
	OpHasSkill
	OpExpRange
)

// Filter 过滤条件，支持任意嵌套的 AND/OR/NOT，叶子为技能或经验区间。
type Filter struct {
	Op       FilterOp
	Children []*Filter // AND/OR/NOT 的子节点；NOT 只取第一个
	Skill    string    // OpHasSkill
	MinExp   int       // OpExpRange 闭区间下界
	MaxExp   int       // OpExpRange 闭区间上界
}

// SkillWeight 岗位加权技能。
type SkillWeight struct {
	Skill  string
	Weight int
}

// Query 一次匹配查询。
type Query struct {
	Pool      string
	Filter    *Filter
	Skills    []SkillWeight
	TargetExp int
	ExpWeight int
	K         int
}

// ScoredCandidate 命中候选人及其得分。
type ScoredCandidate struct {
	ID    string
	Score float64
}
