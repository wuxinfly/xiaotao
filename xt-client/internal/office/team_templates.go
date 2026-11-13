package office

type RoleDraft struct {
	Name           string          `json:"name"`
	Responsibility string          `json:"responsibility"`
	Capabilities   []string        `json:"capabilities"`
	Model          ModelPreference `json:"model"`
	Appearance     Appearance      `json:"appearance"`
}

type TeamTemplate struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Roles       []RoleDraft `json:"roles"`
}

var palette = [][2]string{{"#36a9c9", "#173b55"}, {"#edac53", "#6b422a"}, {"#83b692", "#285c4d"}, {"#bd88c8", "#54365d"}, {"#e48476", "#6f3237"}, {"#889cd7", "#344471"}}
var shapes = []string{"round", "square", "triangle", "hexagon"}

func TeamTemplates() []TeamTemplate {
	return []TeamTemplate{{ID: "product-delivery", Name: "产品实现小队", Description: "讨论需求、设计体验、实现功能并验证交付；不自动派发任务。", Roles: []RoleDraft{
		{Name: "产品", Responsibility: "讨论目标、需求边界、优先级和验收标准", Capabilities: []string{"分析", "规划"}, Appearance: DraftAppearance(0)},
		{Name: "设计", Responsibility: "讨论交互、视觉方向和界面验收", Capabilities: []string{"设计", "评审"}, Appearance: DraftAppearance(1)},
		{Name: "开发", Responsibility: "讨论实现方案、代码质量和交付风险", Capabilities: []string{"开发", "验证"}, Appearance: DraftAppearance(2)},
	}}}
}

func DraftAppearance(index int) Appearance {
	colors := palette[index%len(palette)]
	return Appearance{Shape: shapes[index%len(shapes)], PrimaryColor: colors[0], SecondaryColor: colors[1]}
}
