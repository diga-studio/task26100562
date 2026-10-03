// Package talentpulse 提供 TalentPulse 的候选人匹配库。
// 候选人档案与检索数据保存在进程内存中，由上层直接调用。
package talentpulse

// Candidate 候选人档案。技能与人才池标识已由上游规范化。
type Candidate struct {
	ID             string   // 唯一 ID
	Skills         []string // 技能集合
	ExperienceMths int      // 经验月数
	Pools          []string // 所属人才池集合
}

// FilterOp 过滤条件节点类型。
type FilterOp int

const (
	OpSkill FilterOp = iota // 叶子：持有某技能
	OpExp                   // 叶子：经验月数落在闭区间 [Min, Max]
	OpAnd                   // 全部子条件成立
	OpOr                    // 任一子条件成立
	OpNot                   // 子条件不成立（全集为所选人才池内当前候选人）
)

// Filter 过滤条件，支持任意嵌套的 AND/OR/NOT。
type Filter struct {
	Op     FilterOp
	Skill  string   // OpSkill 使用
	Min    int      // OpExp 使用（含）
	Max    int      // OpExp 使用（含）
	Child  *Filter  // OpNot 使用
	Childs []*Filter // OpAnd / OpOr 使用
}

// WeightedSkill 岗位加权技能，技能互不重复。
type WeightedSkill struct {
	Skill  string
	Weight int // 0..1000
}

// Query 一次匹配查询。
type Query struct {
	Pool         string         // 限定的人才池
	Filter       *Filter        // 过滤条件，只决定是否入选，不参与计分；nil 表示不过滤
	Skills       []WeightedSkill // 岗位加权技能
	TargetExpMth int            // 目标经验月数 1..480
	ExpWeight    int            // 经验权重 0..1000
	K            int            // 返回前 K 名
}

// ScoredCandidate 命中结果。
type ScoredCandidate struct {
	Candidate Candidate
	Score     float64 // 总分 = 技能得分 + 经验得分
}
