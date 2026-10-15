package media

import "fmt"

// Spec 图片访问规格：空/""=原图；缩略图与封面走 CDN 侧处理参数（免服务端生成与存储成本）。
// 处理参数按供应商语法下发（如 OSS `image/resize,w_200`）；新增规格只在此登记。
type Spec struct {
	Name        string
	Desc        string
	ProviderOps string // 供应商处理参数（为空表示原图）
	MaxEdge     int    // 目标最长边（文档口径，便于前端/测试校验）
}

// 一期规格集（PRD 8.9 / 详细设计 4.2.7）。
var specs = map[string]Spec{
	"":        {Name: "orig", Desc: "原图", MaxEdge: 0},
	"orig":    {Name: "orig", Desc: "原图", MaxEdge: 0},
	"thumb_s": {Name: "thumb_s", Desc: "小缩略图（列表）", ProviderOps: "image/resize,m_lfit,w_200", MaxEdge: 200},
	"thumb_m": {Name: "thumb_m", Desc: "中缩略图（卡片）", ProviderOps: "image/resize,m_lfit,w_400", MaxEdge: 400},
	"thumb_l": {Name: "thumb_l", Desc: "大缩略图（详情）", ProviderOps: "image/resize,m_lfit,w_800", MaxEdge: 800},
	"cover":   {Name: "cover", Desc: "封面（16:9 裁切）", ProviderOps: "image/resize,m_fill,w_750,h_422", MaxEdge: 750},
	"avatar":  {Name: "avatar", Desc: "头像（正方裁切）", ProviderOps: "image/resize,m_fill,w_160,h_160", MaxEdge: 160},
}

// SpecNames 全部规格名（后台/前端枚举用）。
func SpecNames() []string {
	out := make([]string, 0, len(specs))
	for k := range specs {
		if k == "" {
			continue
		}
		out = append(out, k)
	}
	return out
}

// ResolveSpec 校验并归一化规格：未知规格直接报错（避免静默返回原图）。
func ResolveSpec(spec string) (Spec, error) {
	s, ok := specs[spec]
	if !ok {
		return Spec{}, fmt.Errorf("media: 未知图片规格 %q", spec)
	}
	return s, nil
}
